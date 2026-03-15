// Package remote resolves remote sources (git URLs, registry packages)
// to local directories for scanning.
package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	forges "github.com/git-pkgs/forge"
	"github.com/git-pkgs/purl"
	"github.com/git-pkgs/registries"
	_ "github.com/git-pkgs/registries/all"
)

// Source describes where a remote project came from.
type Source struct {
	Dir     string // local directory to scan
	Cleanup func() // call when done (removes temp dir unless --keep)
	Origin  string // original URL or package identifier
}

// Options configures remote source resolution.
type Options struct {
	Keep  bool   // don't delete temp dir after scanning
	Depth int    // git clone depth (0 = full, default 1 = shallow)
	Dir   string // directory to clone into (empty = temp dir)
}

// Resolve takes a source string and returns a local directory to scan.
// Handles:
//   - Local paths (returned as-is)
//   - Git URLs (https://github.com/owner/repo)
//   - Registry shorthands (npm:lodash, gem:rails, pypi:requests, crate:serde)
func Resolve(ctx context.Context, source string, opts Options) (*Source, error) {
	if opts.Depth == 0 {
		opts.Depth = 1
	}

	// Check if it's a registry shorthand
	if ecosystem, name, ok := parseRegistryShorthand(source); ok {
		return resolveRegistryPackage(ctx, ecosystem, name, opts)
	}

	// Check if it's a URL
	if isURL(source) {
		return resolveGitURL(ctx, source, opts)
	}

	// Local path
	return &Source{Dir: source, Cleanup: func() {}}, nil
}

// parseRegistryShorthand checks if the source looks like "npm:lodash" or "gem:rails".
func parseRegistryShorthand(source string) (ecosystem, name string, ok bool) {
	parts := strings.SplitN(source, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	prefix := parts[0]
	// Map shorthand prefixes to purl ecosystems
	ecosystems := map[string]string{
		"npm":   "npm",
		"gem":   "gem",
		"pypi":  "pypi",
		"crate": "cargo",
		"go":    "golang",
		"hex":   "hex",
		"nuget": "nuget",
		"pub":   "pub",
	}
	eco, ok := ecosystems[prefix]
	if !ok {
		return "", "", false
	}
	return eco, parts[1], true
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "ssh://")
}

// resolveGitURL clones a git URL to a temp directory.
func resolveGitURL(ctx context.Context, url string, opts Options) (*Source, error) {
	// Use forge to parse and normalize the URL
	domain, owner, repo, err := forges.ParseRepoURL(url)
	if err != nil {
		// Fall back to using the URL directly if it's not a recognized forge URL
		return cloneURL(ctx, url, "", opts)
	}

	cloneURL := fmt.Sprintf("https://%s/%s/%s.git", domain, owner, repo)
	return clone(ctx, cloneURL, repo, opts)
}

// resolveRegistryPackage looks up a package in its registry to find the source repo.
func resolveRegistryPackage(ctx context.Context, ecosystem, name string, opts Options) (*Source, error) {
	purlStr := purl.MakePURLString(ecosystem, name, "")
	client := registries.DefaultClient()
	pkg, err := registries.FetchPackageFromPURL(ctx, purlStr, client)
	if err != nil {
		return nil, fmt.Errorf("looking up %s:%s: %w", ecosystem, name, err)
	}

	if pkg.Repository == "" {
		return nil, fmt.Errorf("%s:%s has no source repository", ecosystem, name)
	}

	return resolveGitURL(ctx, pkg.Repository, opts)
}

func clone(ctx context.Context, url, name string, opts Options) (*Source, error) {
	return cloneURL(ctx, url, name, opts)
}

func cloneURL(ctx context.Context, url, name string, opts Options) (*Source, error) {
	var dir string
	var managed bool

	if opts.Dir != "" {
		dir = opts.Dir
	} else {
		if name == "" {
			name = "brief-remote"
		}
		tmp, err := os.MkdirTemp("", "brief-"+name+"-*")
		if err != nil {
			return nil, fmt.Errorf("creating temp dir: %w", err)
		}
		dir = tmp
		managed = true
	}

	args := []string{"clone"}
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}
	args = append(args, url, dir)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if managed {
			_ = os.RemoveAll(dir)
		}
		return nil, fmt.Errorf("cloning %s: %w", url, err)
	}

	cleanup := func() {
		if managed && !opts.Keep {
			_ = os.RemoveAll(dir)
		}
	}

	return &Source{Dir: dir, Cleanup: cleanup, Origin: url}, nil
}

// TempDir returns the path to the temporary directory, useful for --keep output.
func (s *Source) TempDir() string {
	return filepath.Clean(s.Dir)
}
