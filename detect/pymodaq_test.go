package detect

import "testing"

func TestPyMoDAQProject(t *testing.T) {
	assertToolDetected(t, runOn(t, "../testdata/pymodaq-project"), "library", "PyMoDAQ")
}

func TestPyMoDAQDependencySignals(t *testing.T) {
	for _, tt := range []struct{ path, content string }{
		{"pyproject.toml", "[project]\nname = 'spectrometer'\ndependencies = ['pymodaq>=4']\n"},
		{"requirements.txt", "pymodaq\n"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tt.path, tt.content)
			assertToolDetected(t, runOn(t, dir), "library", "PyMoDAQ")
		})
	}
}

func TestPyMoDAQRequiresDeclaredDependency(t *testing.T) {
	for _, tt := range []struct{ path, content string }{
		{"requirements.txt", "pymodaq-plugins\n"},
		{"README.md", "Install pymodaq to control the spectrometer.\n"},
		{"instrument.py", "from pymodaq import utils\n"},
		{"fixtures/requirements.txt", "pymodaq\n"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, tt.path, tt.content)
			assertToolNotDetected(t, runOn(t, dir), "library", "PyMoDAQ")
		})
	}
}
