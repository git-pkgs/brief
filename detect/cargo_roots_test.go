package detect

import (
	"fmt"
	"path"
	"testing"
)

func TestIndependentCargoRoots(t *testing.T) {
	for _, rootManifest := range []bool{false, true} {
		t.Run(fmt.Sprintf("root=%t", rootManifest), func(t *testing.T) {
			dir := t.TempDir()
			roots := []string{"bindings/node", "bindings/python", "core"}
			if rootManifest {
				roots = append(roots, "")
			}
			for i, root := range roots {
				writeFile(t, dir, path.Join(root, "Cargo.toml"), fmt.Sprintf("[package]\nname = \"project%d\"\nversion = \"0.1.0\"\n", i))
				writeFile(t, dir, path.Join(root, "src/lib.rs"), "pub fn example() {}\n")
				writeFile(t, dir, path.Join(root, "Cargo.lock"), fmt.Sprintf("version = 4\n[[package]]\nname = \"dep%d\"\nversion = \"1.0.0\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n", i))
			}
			report := runOn(t, dir)
			seen := make(map[string]int)
			for _, manifest := range report.Manifests {
				seen[manifest.Path]++
			}
			deps := make(map[string]bool)
			for _, dep := range report.Dependencies {
				deps[dep.Name] = true
			}
			for i, root := range roots {
				for _, file := range []string{"Cargo.toml", "Cargo.lock"} {
					want := path.Join(root, file)
					if seen[want] != 1 {
						t.Errorf("manifest %q occurs %d times", want, seen[want])
					}
				}
				if !deps[fmt.Sprintf("dep%d", i)] {
					t.Errorf("missing dependencies from %q", root)
				}
			}
		})
	}
}

func TestIndependentCargoRootsHonorScanBounds(t *testing.T) {
	dir := t.TempDir()
	for _, root := range []string{"core", "bindings/python", "target/generated"} {
		writeFile(t, dir, path.Join(root, "Cargo.toml"), "[package]\nname = \"example\"\nversion = \"0.1.0\"\n")
		writeFile(t, dir, path.Join(root, "lib.rs"), "pub fn example() {}\n")
	}
	engine := New(loadKB(t), dir)
	engine.ScanDepth = 1
	report, err := engine.Run()
	if err != nil {
		t.Fatal(err)
	}
	for _, manifest := range report.Manifests {
		if manifest.Path != "core/Cargo.toml" {
			t.Errorf("unexpected manifest beyond scan bounds: %q", manifest.Path)
		}
	}
	if len(report.Manifests) != 1 {
		t.Errorf("manifests = %+v", report.Manifests)
	}
}
