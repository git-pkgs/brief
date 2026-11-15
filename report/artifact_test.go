package report

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/brief/binary"
)

func TestArtifactMarkdown(t *testing.T) {
	art := &brief.Artifact{
		Version: "dev", Path: "package.whl", Format: "zip", SHA256: "abc123", Entries: 3,
		DurationMS: 1.25,
		NativeObjects: []binary.Object{
			{
				Path: "lib/example.so", Format: "elf", Arch: "amd64", SOName: "example.so.1",
				Producer: []string{"clang", "rustc"}, Needed: []string{"libc.so.6", "libm.so.6"},
				Go: &binary.GoBuild{
					Version: "go1.27.3", Path: "example.org/tool", Main: "example.org/main v1.0.0",
					Deps: []string{"example.org/dep v2.0.0"},
				},
				Static: []binary.Hint{{Library: "zlib", Version: "1.3", Match: "zlib 1.3"}},
			},
			{Path: "bin/tool.exe", Format: "pe", Arch: "arm64"},
		},
	}
	var out bytes.Buffer
	ArtifactMarkdown(&out, art)
	for _, want := range []string{
		"# brief dev — package.whl", "**Format:** zip", "**SHA256:** abc123",
		"**Entries:** 3 files, 2 native objects", "## Object: lib/example.so", "**Format:** elf",
		"**Architecture:** amd64", "**SOName:** example.so.1", "**Producers:**\n\n- clang\n- rustc",
		"**Needed:**\n\n- libc.so.6\n- libm.so.6",
		"**Go:** go1.27.3", "**Go path:** example.org/tool", "**Go main:** example.org/main v1.0.0",
		"**Go dependencies:**\n\n- example.org/dep v2.0.0", "**Static (low confidence):** zlib 1.3",
		"**Matched text:** zlib 1.3", "## Object: bin/tool.exe", "**Format:** pe",
		"**Architecture:** arm64", "1.2ms",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in Markdown:\n%s", want, out.String())
		}
	}
}

func TestArtifactMarkdownEmpty(t *testing.T) {
	var out bytes.Buffer
	ArtifactMarkdown(&out, &brief.Artifact{Path: "empty.zip", Format: "zip"})
	for _, unwanted := range []string{"**Entries:**", "## Object:", "**SHA256:**", "**Go:"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("unexpected optional field %q: %s", unwanted, out.String())
		}
	}
}

func TestArtifactMarkdownBareObject(t *testing.T) {
	var out bytes.Buffer
	ArtifactMarkdown(&out, &brief.Artifact{
		Path: "build/tool", Format: "elf",
		NativeObjects: []binary.Object{{Path: "build/tool", Format: "elf", Arch: "amd64"}},
	})
	if strings.Contains(out.String(), "**Entries:**") {
		t.Errorf("bare binary reports archive entries: %s", out.String())
	}
	if !strings.Contains(out.String(), "## Object: build/tool") {
		t.Errorf("bare binary is missing its object metadata: %s", out.String())
	}
}

func TestArtifactMarkdownLists(t *testing.T) {
	for _, count := range []int{0, 1, maxDisplayItems, maxDisplayItems + 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			values := make([]string, count)
			for i := range values {
				values[i] = fmt.Sprintf("item-%02d", i)
			}
			var out bytes.Buffer
			ArtifactMarkdown(&out, &brief.Artifact{NativeObjects: []binary.Object{{
				Producer: values, Needed: values, Go: &binary.GoBuild{Deps: values},
			}}})
			for _, header := range []string{"Producers", "Needed", "Go dependencies"} {
				if got := strings.Contains(out.String(), "**"+header+":**\n\n- item-00"); got != (count > 0) {
					t.Errorf("%s list presence = %v for %d values:\n%s", header, got, count, out.String())
				}
			}
			for i, value := range values {
				want := 3
				if i >= maxDisplayItems {
					want = 0
				}
				if got := strings.Count(out.String(), "- "+value+"\n"); got != want {
					t.Errorf("%s occurs %d times, want %d", value, got, want)
				}
			}
			wantTruncated := 0
			if count > maxDisplayItems {
				wantTruncated = 3
			}
			if got := strings.Count(out.String(), "- ... and 3 more\n"); got != wantTruncated {
				t.Errorf("truncation summaries = %d, want %d:\n%s", got, wantTruncated, out.String())
			}
		})
	}
}

func TestArtifactMarkdownEscapesMetadata(t *testing.T) {
	value := "[link](url)|`*_<script>\n## forged\r\tend\u202e\u2066\u2069\u200b"
	art := &brief.Artifact{
		Version: value, Path: value, Format: value, SHA256: value,
		NativeObjects: []binary.Object{{
			Path: value, Format: value, Arch: value, SOName: value,
			Producer: []string{value}, Needed: []string{value},
			Go:     &binary.GoBuild{Version: value, Path: value, Main: value, Deps: []string{value}},
			Static: []binary.Hint{{Library: value, Version: value, Match: value}},
		}},
	}
	var out bytes.Buffer
	ArtifactMarkdown(&out, art)
	want := "\\[link\\](url)\\|\\`\\*\\_&lt;script&gt; ## forged end"
	if n := strings.Count(out.String(), want); n != 17 {
		t.Errorf("escaped metadata occurrences = %d, want 17:\n%s", n, out.String())
	}
	for _, unwanted := range []string{"[link](url)", "<script>", "\n## forged", "\r", "\t", "\u202e", "\u2066", "\u2069", "\u200b"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("unescaped metadata %q: %s", unwanted, out.String())
		}
	}
}
