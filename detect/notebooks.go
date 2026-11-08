package detect

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/kb"
)

var errNotebookFormat = errors.New("expected a version 3 or 4 notebook with code-cell sources")

const (
	notebookFileBytes     = 8 << 20
	notebookTotalBytes    = 32 << 20
	notebookFileCount     = 32
	notebookFormat        = 4
	notebookLegacy        = 3
	notebookInvalid       = "invalid"
	notebookLanguageBytes = 64
)

type notebookCell struct {
	Type    string          `json:"cell_type"`
	Source  json.RawMessage `json:"source"`
	Input   json.RawMessage `json:"input"`
	Outputs json.RawMessage `json:"outputs"`
}

type notebookDocument struct {
	Format   int `json:"nbformat"`
	Metadata struct {
		Language   string `json:"language"`
		KernelSpec struct {
			Language string `json:"language"`
		} `json:"kernelspec"`
		LanguageInfo struct {
			Name string `json:"name"`
		} `json:"language_info"`
	} `json:"metadata"`
	Cells      []notebookCell `json:"cells"`
	Worksheets []struct {
		Cells []notebookCell `json:"cells"`
	} `json:"worksheets"`
}

func (e *Engine) detectNotebooks() *brief.NotebookInfo {
	e.loadProjectFiles()
	result := &brief.NotebookInfo{Files: []brief.NotebookFile{}}
	for _, file := range e.indexedFiles {
		if !strings.EqualFold(filepath.Ext(file), ".ipynb") {
			continue
		}
		if len(result.Files) >= notebookFileCount {
			result.SkippedFiles++
			continue
		}
		stats, read := e.readNotebook(file, notebookTotalBytes-result.ReadBytes)
		result.Files = append(result.Files, stats)
		result.ReadBytes += read
	}
	return result
}

func (e *Engine) readNotebook(file string, remaining int64) (brief.NotebookFile, int64) {
	stats := brief.NotebookFile{Path: filepath.ToSlash(file), Status: "unreadable"}
	f, err := e.projectFS().Open(filepath.ToSlash(file))
	if err != nil {
		return stats, 0
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return stats, 0
	}
	stats.FileBytes = info.Size()
	if info.Size() > notebookFileBytes {
		stats.Status = "size_limit"
		return stats, 0
	}
	if info.Size() > remaining {
		stats.Status = "byte_limit"
		return stats, 0
	}
	limit := min(int64(notebookFileBytes), remaining)
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	read := int64(len(data))
	if err != nil {
		return stats, read
	}
	if read > limit {
		stats.Status = "size_limit"
		return stats, read
	}
	parsed, err := parseNotebook(data)
	if err != nil {
		stats.Status = notebookInvalid
		return stats, read
	}
	parsed.Path, parsed.FileBytes, parsed.Status = stats.Path, stats.FileBytes, "ok"
	return parsed, read
}

func parseNotebook(data []byte) (brief.NotebookFile, error) {
	var document notebookDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return brief.NotebookFile{}, err
	}
	cells, err := notebookCells(document)
	if err != nil {
		return brief.NotebookFile{}, err
	}
	result := brief.NotebookFile{}
	for _, candidate := range []struct{ value, source string }{
		{document.Metadata.LanguageInfo.Name, "language_info"},
		{document.Metadata.KernelSpec.Language, "kernelspec"},
		{document.Metadata.Language, "metadata"},
	} {
		language := strings.TrimSpace(candidate.value)
		if language != "" && len(language) <= notebookLanguageBytes {
			result.Language, result.LanguageSource = language, candidate.source
			break
		}
	}
	for _, cell := range cells {
		if cell.Type != "code" {
			continue
		}
		source, err := notebookSource(cell.Source)
		if err != nil {
			return brief.NotebookFile{}, err
		}
		result.CodeCells++
		result.CodeBytes += len(source)
		result.CodeLines += strings.Count(source, "\n")
		if source != "" && !strings.HasSuffix(source, "\n") {
			result.CodeLines++
		}
		result.OutputBytes += len(cell.Outputs)
	}
	return result, nil
}

func notebookCells(document notebookDocument) ([]notebookCell, error) {
	if document.Format == notebookFormat && document.Cells != nil {
		return document.Cells, nil
	}
	if document.Format != notebookLegacy || document.Worksheets == nil {
		return nil, errNotebookFormat
	}
	var cells []notebookCell
	for _, worksheet := range document.Worksheets {
		for _, cell := range worksheet.Cells {
			cell.Source = cell.Input
			cells = append(cells, cell)
		}
	}
	return cells, nil
}

func notebookSource(data json.RawMessage) (string, error) {
	var single string
	if len(data) > 0 && data[0] == '"' {
		err := json.Unmarshal(data, &single)
		return single, err
	}
	var lines []string
	if err := json.Unmarshal(data, &lines); err != nil {
		return "", err
	}
	if lines == nil {
		return "", errNotebookFormat
	}
	return strings.Join(lines, ""), nil
}

func (e *Engine) notebookLanguagePaths(language string) []string {
	if e.notebooks == nil || !e.NotebookStats {
		return nil
	}
	var paths []string
	for _, file := range e.notebooks.Files {
		if file.Status == "ok" && file.CodeCells > 0 && strings.EqualFold(file.Language, language) && e.languageEvidence(file.Path) {
			paths = append(paths, file.Path)
		}
	}
	return paths
}

func (e *Engine) notebookLanguageConfidence(tool *kb.ToolDef) brief.Confidence {
	if tool.Tool.Category == categoryLanguage && len(e.notebookLanguagePaths(tool.Tool.Name)) > 0 {
		return brief.ConfidenceHigh
	}
	return ""
}
