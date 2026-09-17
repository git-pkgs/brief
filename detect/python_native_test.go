package detect

import "testing"

func TestPythonNativeExtensionFixtures(t *testing.T) {
	for _, tt := range []struct{ fixture, tool string }{
		{"pybind11-project", "pybind11"},
		{"nanobind-project", "nanobind"},
		{"scikit-build-core-project", "scikit-build-core"},
		{"cffi-project", "cffi"},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			report := runOn(t, "../testdata/"+tt.fixture)
			assertHighConfidenceToolDetected(t, report, "native_extension", tt.tool)
		})
	}
}

func TestPythonNativeExtensionSignals(t *testing.T) {
	for _, tt := range []struct{ tool, file, content string }{
		{"pybind11", "pyproject.toml", "[build-system]\nrequires = ['pybind11 >=2.13']\n"},
		{"nanobind", "pyproject.toml", "[build-system]\nrequires = ['nanobind']\n"},
		{"pybind11", "CMakeLists.txt", "find_package(pybind11 CONFIG REQUIRED)\n"},
		{"pybind11", "CMakeLists.txt", "pybind11_add_module(example src.cpp)\n"},
		{"nanobind", "CMakeLists.txt", "find_package(nanobind CONFIG REQUIRED)\n"},
		{"nanobind", "CMakeLists.txt", "nanobind_add_module(example src.cpp)\n"},
		{"scikit-build-core", "pyproject.toml", "[build-system]\nbuild-backend='scikit_build_core.build'\n"},
		{"cffi", "setup.py", "setup(cffi_modules = ['build.py:ffibuilder'])\n"},
	} {
		t.Run(tt.tool+"/"+tt.content, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "example.py", "")
			writeFile(t, dir, tt.file, tt.content)
			assertHighConfidenceToolDetected(t, runOn(t, dir), "native_extension", tt.tool)
		})
	}
}

func TestPythonNativeExtensionNearMiss(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = 'plain'\nversion = '0.1.0'\ndependencies = ['cffi>=1', 'pybind11-stubgen', 'nanobind-example']\n")
	writeFile(t, dir, "CMakeLists.txt", "find_package(Python COMPONENTS Interpreter REQUIRED)\n")
	report := runOn(t, dir)
	for _, tool := range report.Tools["native_extension"] {
		t.Errorf("unexpected native-extension tool: %s", tool.Name)
	}
}
