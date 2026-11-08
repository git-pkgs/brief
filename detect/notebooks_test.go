package detect

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/git-pkgs/brief"
)

func TestNotebookStatisticsThroughScan(t *testing.T) {
	content := `{"nbformat":4,"metadata":{"language_info":{"name":"python"}},"cells":[
		{"cell_type":"markdown","source":["not source"]},
		{"cell_type":"code","source":["π = 3\n","print(π)"],"outputs":[{"text":"saved output"}]},
		{"cell_type":"code","source":"pass\n","outputs":[]}]}`
	engine := NewFS(loadKB(t), fstest.MapFS{"analysis.IPYNB": {Data: []byte(content)}})
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil {
		t.Fatal(err)
	}
	got := report.Notebooks.Files[0]
	if got.Status != "ok" || got.Language != "python" || got.CodeCells != 2 || got.CodeLines != 3 || got.CodeBytes != len("π = 3\nprint(π)pass\n") {
		t.Fatalf("unexpected notebook statistics: %+v", got)
	}
	if got.FileBytes != int64(len(content)) || got.OutputBytes == 0 {
		t.Fatalf("missing byte counts: %+v", got)
	}
	if len(report.Languages) != 1 || report.Languages[0].Name != "Python" || report.Languages[0].Confidence != brief.ConfidenceHigh || report.Languages[0].ConfigFiles[0] != "analysis.IPYNB" {
		t.Fatalf("kernel language missing from detections: %+v", report.Languages)
	}
	encoded, err := json.Marshal(report.Notebooks)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "saved output") || strings.Contains(string(encoded), "print(") {
		t.Fatal("statistics contain notebook contents")
	}
}

func TestNotebookStatisticsAreOptInAndResetSafe(t *testing.T) {
	engine := NewFS(loadKB(t), fstest.MapFS{"a.ipynb": {Data: []byte(`{"nbformat":4,"cells":[]}`)}})
	report, err := engine.Run()
	if err != nil || report.Notebooks != nil {
		t.Fatalf("default scan: %+v, %v", report.Notebooks, err)
	}
	engine.NotebookStats = true
	for range 2 {
		report, err = engine.Run()
		if err != nil || report.Notebooks == nil || len(report.Notebooks.Files) != 1 {
			t.Fatalf("enabled scan: %+v, %v", report.Notebooks, err)
		}
	}
	engine.resetScanState()
	if !engine.NotebookStats {
		t.Fatal("scan reset discarded the notebook option")
	}
}

func TestNotebookStatisticsReportInvalidSourcesAndFormats(t *testing.T) {
	cases := []string{`{`, `{"nbformat":2,"cells":[]}`, `{"nbformat":4}`, `{"nbformat":4,"cells":null}`,
		`{"nbformat":4,"cells":[{"cell_type":"code","source":[4]}]}`,
		`{"nbformat":4,"cells":[{"cell_type":"code"}]}`}
	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			engine := NewFS(loadKB(t), fstest.MapFS{"a.ipynb": {Data: []byte(content)}})
			engine.NotebookStats = true
			report, err := engine.Run()
			if err != nil || report.Notebooks.Files[0].Status != "invalid" {
				t.Fatalf("invalid notebook was accepted: %+v, %v", report, err)
			}
		})
	}
}

func TestLegacyNotebookStatisticsThroughScan(t *testing.T) {
	content := `{"nbformat":3,"worksheets":[{"cells":[{"cell_type":"code","input":["print(1)\n"],"outputs":[{"text":"saved output"}]}]}]}`
	engine := NewFS(loadKB(t), fstest.MapFS{"old.ipynb": {Data: []byte(content)}})
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil || report.Notebooks.Files[0].Status != "ok" || report.Notebooks.Files[0].CodeLines != 1 {
		t.Fatalf("legacy notebook not parsed: %+v, %v", report.Notebooks, err)
	}
}

func TestNotebookKernelLanguageFallbacks(t *testing.T) {
	cases := []struct {
		metadata string
		language string
		source   string
	}{
		{`"language_info":{"name":"R"},"kernelspec":{"language":"python"}`, "R", "language_info"},
		{`"kernelspec":{"language":"julia"}`, "julia", "kernelspec"},
		{`"language":"python"`, "python", "metadata"},
		{`"language_info":{"name":"` + strings.Repeat("x", notebookLanguageBytes+1) + `"},"kernelspec":{"language":"python"}`, "python", "kernelspec"},
	}
	for _, tc := range cases {
		t.Run(tc.source+tc.language, func(t *testing.T) {
			content := `{"nbformat":4,"metadata":{` + tc.metadata + `},"cells":[{"cell_type":"code","source":["x\n"]}]}`
			engine := NewFS(loadKB(t), fstest.MapFS{"a.ipynb": {Data: []byte(content)}})
			engine.NotebookStats = true
			report, err := engine.Run()
			if err != nil {
				t.Fatal(err)
			}
			file := report.Notebooks.Files[0]
			if file.Language != tc.language || file.LanguageSource != tc.source || len(report.Languages) != 1 {
				t.Fatalf("language metadata mismatch: %+v, %+v", file, report.Languages)
			}
		})
	}
}

func TestNotebookMarkdownDoesNotEstablishCodeLanguage(t *testing.T) {
	content := `{"nbformat":4,"metadata":{"language_info":{"name":"python"}},"cells":[{"cell_type":"markdown","source":["A description"]}]}`
	engine := NewFS(loadKB(t), fstest.MapFS{"a.ipynb": {Data: []byte(content)}})
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil || len(report.Languages) != 0 || report.Notebooks.Files[0].CodeCells != 0 {
		t.Fatalf("markdown-only notebook established a code language: %+v, %v", report, err)
	}
}

func TestNotebookDocumentationDoesNotEstablishProjectLanguage(t *testing.T) {
	content := `{"nbformat":4,"metadata":{"language_info":{"name":"python"}},"cells":[{"cell_type":"code","source":["print(1)\n"]}]}`
	engine := NewFS(loadKB(t), fstest.MapFS{"docs/example.ipynb": {Data: []byte(content)}})
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil || len(report.Languages) != 0 || len(report.Notebooks.Files) != 1 {
		t.Fatalf("documentation notebook established a project language: %+v, %v", report, err)
	}
}

func TestNotebookStatisticsRespectFileAndByteLimits(t *testing.T) {
	files := fstest.MapFS{}
	for i := range notebookFileCount + 2 {
		files[fmt.Sprintf("%02d.ipynb", i)] = &fstest.MapFile{Data: []byte(`{"nbformat":4,"cells":[]}`)}
	}
	files["large.ipynb"] = &fstest.MapFile{Data: []byte(strings.Repeat(" ", notebookFileBytes+1))}
	engine := NewFS(loadKB(t), files)
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil || len(report.Notebooks.Files) != notebookFileCount || report.Notebooks.SkippedFiles != 3 {
		t.Fatalf("file cap not enforced: %+v, %v", report.Notebooks, err)
	}
	stats, read := engine.readNotebook("large.ipynb", notebookTotalBytes)
	if stats.Status != "size_limit" || read != 0 {
		t.Fatalf("oversized notebook was read: %+v, %d", stats, read)
	}
	stats, read = engine.readNotebook("00.ipynb", 1)
	if stats.Status != "byte_limit" || read != 0 {
		t.Fatalf("byte cap not enforced: %+v, %d", stats, read)
	}
}

func TestNotebookStatisticsRespectScanExclusions(t *testing.T) {
	files := fstest.MapFS{"keep/a.ipynb": {Data: []byte(`{"nbformat":4,"cells":[]}`)},
		"skip/b.ipynb": {Data: []byte(`{"nbformat":4,"cells":[]}`)}, "old.nb": {Data: []byte("Wolfram notebook")}}
	engine := NewFS(loadKB(t), files)
	engine.SkipDirs = []string{"skip"}
	engine.NotebookStats = true
	report, err := engine.Run()
	if err != nil || len(report.Notebooks.Files) != 1 || report.Notebooks.Files[0].Path != "keep/a.ipynb" {
		t.Fatalf("exclusions not respected: %+v, %v", report.Notebooks, err)
	}
}
