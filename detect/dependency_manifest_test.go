package detect

import (
	"encoding/json"
	"testing"
)

func TestDependencyManifestPaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", `[workspace]
members = ["crates/app"]
`)
	writeFile(t, dir, "crates/app/Cargo.toml", `[package]
name = "app"
version = "0.1.0"
[dependencies]
serde = "1"
`)
	for _, path := range []string{"Cargo.lock", "crates/app/Cargo.lock"} {
		writeFile(t, dir, path, `version = 4
[[package]]
name = "serde"
version = "1.0.219"
source = "registry+https://github.com/rust-lang/crates.io-index"
`)
	}
	report := runOn(t, dir)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Dependencies []struct {
			Name     string
			Manifest string
		}
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, dep := range output.Dependencies {
		if dep.Name == "serde" {
			seen[dep.Manifest]++
		}
	}
	for _, path := range []string{"Cargo.lock", "crates/app/Cargo.toml", "crates/app/Cargo.lock"} {
		if seen[path] != 1 {
			t.Errorf("expected one serde row from %q, got %v", path, seen)
		}
	}
	if seen[""] != 0 {
		t.Errorf("dependencies have no manifest: %s", data)
	}
}
