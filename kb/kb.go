// Package kb loads and queries the embedded TOML knowledge base.
package kb

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ToolDef is the parsed representation of a tool TOML file.
type ToolDef struct {
	Tool     ToolInfo    `toml:"tool"`
	Detect   DetectInfo  `toml:"detect"`
	Commands CommandInfo `toml:"commands"`
	Config   ConfigInfo  `toml:"config"`
}

// ToolInfo holds metadata about a tool.
type ToolInfo struct {
	Name        string `toml:"name"`
	Category    string `toml:"category"`
	Homepage    string `toml:"homepage"`
	Docs        string `toml:"docs"`
	Repo        string `toml:"repo"`
	Description string `toml:"description"`
}

// DetectInfo holds the detection primitives for a tool.
type DetectInfo struct {
	Files           []string            `toml:"files"`
	Dependencies    []string            `toml:"dependencies"`
	DevDependencies []string            `toml:"dev_dependencies"`
	FileContains    map[string][]string `toml:"file_contains"`
	KeyExists       map[string][]string `toml:"key_exists"`
	Ecosystems      []string            `toml:"ecosystems"`
}

// CommandInfo holds the commands associated with a tool.
type CommandInfo struct {
	Run          string   `toml:"run"`
	Alternatives []string `toml:"alternatives"`
}

// ConfigInfo holds paths to a tool's configuration files.
type ConfigInfo struct {
	Files    []string `toml:"files"`
	Lockfile string   `toml:"lockfile"`
}

// ScriptSourceDef defines how to extract scripts from a project file.
type ScriptSourceDef struct {
	Source SourceInfo `toml:"source"`
}

// SourceInfo holds metadata about a script source.
type SourceInfo struct {
	Name   string `toml:"name"`
	File   string `toml:"file"`
	Format string `toml:"format"` // "makefile", "json_scripts", "toml_scripts", "justfile"
}

// ResourceDef defines a project resource to look for.
type ResourceDef struct {
	Resource ResourceInfo `toml:"resource"`
}

// ResourceInfo holds the detection rules for a resource file.
type ResourceInfo struct {
	Name     string   `toml:"name"`
	Field    string   `toml:"field"`    // JSON field name in output
	Patterns []string `toml:"patterns"` // file patterns to match
}

// LayoutDef defines directory layout patterns to detect.
type LayoutDef struct {
	Layout LayoutRules `toml:"layout"`
}

// LayoutRules holds the layout detection rules.
type LayoutRules struct {
	SourceDirs []string `toml:"source_dirs"` // directories that indicate source
	TestDirs   []string `toml:"test_dirs"`   // directories that indicate tests
}

// StyleConfigDef defines style configuration files to check.
type StyleConfigDef struct {
	Style StyleRules `toml:"style"`
}

// StyleRules holds style detection rules.
type StyleRules struct {
	ConfigFiles []StyleConfigFile `toml:"config_files"`
	SampleExts  []string          `toml:"sample_extensions"` // extensions to sample for inference
	SampleLimit int               `toml:"sample_limit"`
}

// StyleConfigFile maps a config file to the style info it provides.
type StyleConfigFile struct {
	File       string `toml:"file"`
	Provides   string `toml:"provides"` // what it configures: "indentation", "all", etc.
	SourceName string `toml:"source_name"`
}

// RuntimeVersionDef defines runtime version files to detect.
type RuntimeVersionDef struct {
	Runtime RuntimeInfo `toml:"runtime"`
}

// RuntimeInfo holds detection rules for runtime version files.
type RuntimeInfo struct {
	Name  string   `toml:"name"`  // e.g. "Ruby", "Node"
	Files []string `toml:"files"` // e.g. [".ruby-version", ".tool-versions"]
}

// CIConfigDef defines CI configuration for matrix parsing.
type CIConfigDef struct {
	CI CIConfig `toml:"ci"`
}

// CIConfig holds CI parsing rules.
type CIConfig struct {
	Format     string            `toml:"format"`
	Files      []CIFilePattern   `toml:"files"`
	MatrixKeys map[string]string `toml:"matrix_keys"` // maps our key name to the CI matrix key
}

// CIFilePattern defines a file pattern for CI configs.
type CIFilePattern struct {
	Pattern string `toml:"pattern"`
}

// ManifestFileDef defines manifest files the dependency checker should look at.
type ManifestFileDef struct {
	Manifests ManifestInfo `toml:"manifests"`
}

// ManifestInfo lists manifest files for dependency detection.
type ManifestInfo struct {
	Files []string `toml:"files"`
}

// KnowledgeBase holds all loaded definitions.
type KnowledgeBase struct {
	Tools          []ToolDef
	ByName         map[string]*ToolDef
	ByCategory     map[string][]*ToolDef
	Ecosystems     map[string][]*ToolDef
	ScriptSources  []ScriptSourceDef
	Resources      []ResourceDef
	Layouts        *LayoutDef
	StyleConfig    *StyleConfigDef
	Runtimes       []RuntimeVersionDef
	ManifestFiles  []string
	CIConfig       *CIConfigDef
}

// Load reads all TOML files from the embedded filesystem and returns a KnowledgeBase.
func Load(fsys embed.FS) (*KnowledgeBase, error) {
	base := &KnowledgeBase{
		ByName:     make(map[string]*ToolDef),
		ByCategory: make(map[string][]*ToolDef),
		Ecosystems: make(map[string][]*ToolDef),
	}

	err := fs.WalkDir(fsys, "knowledge", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".toml" {
			return nil
		}

		data, err := fsys.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		name := filepath.Base(path)

		// Route to the right parser based on filename convention
		switch {
		case name == "_scripts.toml":
			return base.loadScriptSource(data, path)
		case name == "_resources.toml":
			return base.loadResources(data, path)
		case name == "_layout.toml":
			return base.loadLayout(data, path)
		case name == "_style.toml":
			return base.loadStyle(data, path)
		case name == "_runtimes.toml":
			return base.loadRuntimes(data, path)
		case name == "_manifests.toml":
			return base.loadManifests(data, path)
		case name == "_ci.toml":
			return base.loadCIConfig(data, path)
		default:
			return base.loadTool(data, path)
		}
	})
	if err != nil {
		return nil, err
	}

	return base, nil
}

func (base *KnowledgeBase) loadTool(data []byte, path string) error {
	var def ToolDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	dir := filepath.Dir(path)
	ecosystem := filepath.Base(dir)

	base.Tools = append(base.Tools, def)
	ptr := &base.Tools[len(base.Tools)-1]
	base.ByName[def.Tool.Name] = ptr
	base.ByCategory[def.Tool.Category] = append(base.ByCategory[def.Tool.Category], ptr)

	if ecosystem != "_shared" {
		base.Ecosystems[ecosystem] = append(base.Ecosystems[ecosystem], ptr)
	}
	for _, eco := range def.Detect.Ecosystems {
		if eco != ecosystem {
			base.Ecosystems[eco] = append(base.Ecosystems[eco], ptr)
		}
	}

	return nil
}

func (base *KnowledgeBase) loadScriptSource(data []byte, path string) error {
	var def ScriptSourceDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.ScriptSources = append(base.ScriptSources, def)
	return nil
}

func (base *KnowledgeBase) loadResources(data []byte, path string) error {
	// Resources file contains an array of resource definitions
	var wrapper struct {
		Resources []ResourceDef `toml:"resources"`
	}
	if err := toml.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.Resources = append(base.Resources, wrapper.Resources...)
	return nil
}

func (base *KnowledgeBase) loadLayout(data []byte, path string) error {
	var def LayoutDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.Layouts = &def
	return nil
}

func (base *KnowledgeBase) loadStyle(data []byte, path string) error {
	var def StyleConfigDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.StyleConfig = &def
	return nil
}

func (base *KnowledgeBase) loadRuntimes(data []byte, path string) error {
	var wrapper struct {
		Runtimes []RuntimeVersionDef `toml:"runtimes"`
	}
	if err := toml.Unmarshal(data, &wrapper); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.Runtimes = append(base.Runtimes, wrapper.Runtimes...)
	return nil
}

func (base *KnowledgeBase) loadManifests(data []byte, path string) error {
	var def ManifestFileDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.ManifestFiles = append(base.ManifestFiles, def.Manifests.Files...)
	return nil
}

func (base *KnowledgeBase) loadCIConfig(data []byte, path string) error {
	var def CIConfigDef
	if err := toml.Unmarshal(data, &def); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	base.CIConfig = &def
	return nil
}

// ToolsForCategory returns all tools matching a category.
func (base *KnowledgeBase) ToolsForCategory(category string) []*ToolDef {
	return base.ByCategory[category]
}

// ToolsForEcosystem returns all tools for a given ecosystem.
func (base *KnowledgeBase) ToolsForEcosystem(ecosystem string) []*ToolDef {
	return base.Ecosystems[ecosystem]
}

// Categories returns all known tool categories.
func (base *KnowledgeBase) Categories() []string {
	cats := make([]string, 0, len(base.ByCategory))
	for c := range base.ByCategory {
		cats = append(cats, c)
	}
	return cats
}

// AllEcosystems returns all known ecosystem names.
func (base *KnowledgeBase) AllEcosystems() []string {
	ecos := make([]string, 0, len(base.Ecosystems))
	for e := range base.Ecosystems {
		ecos = append(ecos, e)
	}
	return ecos
}

// HasGlobPattern checks if a pattern contains glob characters.
func HasGlobPattern(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}
