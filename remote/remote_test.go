package remote

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestResolveLocalPath(t *testing.T) {
	source, err := Resolve(context.Background(), "./project", Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if source.Dir != "./project" {
		t.Errorf("Dir = %q", source.Dir)
	}
	if source.Origin != "" {
		t.Errorf("Origin = %q", source.Origin)
	}
}

func TestResolveHTTPSDefaultShallowAndCleanup(t *testing.T) {
	originalEnsure := ensureClone
	t.Cleanup(func() { ensureClone = originalEnsure })

	var clonedDir string
	ensureClone = func(_ context.Context, url, dst string, full bool) error {
		if url != "https://github.com/example/project" {
			t.Errorf("url = %q", url)
		}
		if full {
			t.Error("default clone should be shallow")
		}
		clonedDir = dst
		return nil
	}

	source, err := Resolve(context.Background(), "https://github.com/example/project", Options{Depth: -1})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if source.Dir != clonedDir {
		t.Errorf("Dir = %q, cloned into %q", source.Dir, clonedDir)
	}
	if source.Origin != "https://github.com/example/project" {
		t.Errorf("Origin = %q", source.Origin)
	}
	if _, err := os.Stat(source.Dir); err != nil {
		t.Fatalf("temporary clone directory: %v", err)
	}
	source.Cleanup()
	if _, err := os.Stat(source.Dir); !os.IsNotExist(err) {
		t.Errorf("temporary clone directory remains after cleanup: %v", err)
	}
}

func TestResolveHTTPSRemovesTemporaryDirectoryOnFailure(t *testing.T) {
	originalEnsure := ensureClone
	t.Cleanup(func() { ensureClone = originalEnsure })

	var clonedDir string
	ensureClone = func(_ context.Context, _, dst string, _ bool) error {
		clonedDir = dst
		return errors.New("clone failed")
	}

	_, err := Resolve(context.Background(), "https://github.com/example/project", Options{})
	if err == nil || !strings.Contains(err.Error(), "clone failed") {
		t.Fatalf("Resolve error = %v", err)
	}
	if _, err := os.Stat(clonedDir); !os.IsNotExist(err) {
		t.Errorf("temporary clone directory remains after failure: %v", err)
	}
}

func TestHTTPSCloneDepthMapping(t *testing.T) {
	originalEnsure := ensureClone
	originalCache := prepareCloneCache
	originalExec := execClone
	t.Cleanup(func() {
		ensureClone = originalEnsure
		prepareCloneCache = originalCache
		execClone = originalExec
	})

	execClone = func(context.Context, string, string, int) error {
		return errors.New("unexpected exec clone")
	}
	prepareCloneCache = func(context.Context, string, string, string) error {
		return errors.New("unexpected cache clone")
	}

	for _, tt := range []struct {
		name  string
		depth int
		full  bool
	}{
		{name: "full", depth: 0, full: true},
		{name: "shallow", depth: 1, full: false},
		{name: "positive depth is shallow", depth: 20, full: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			ensureClone = func(_ context.Context, url, dst string, full bool) error {
				called = true
				if url != "https://example.com/owner/repo" {
					t.Errorf("url = %q", url)
				}
				if dst != "/tmp/brief-checkout" {
					t.Errorf("dst = %q", dst)
				}
				if full != tt.full {
					t.Errorf("full = %v, want %v", full, tt.full)
				}
				return nil
			}

			err := cloneInto(
				context.Background(),
				"https://example.com/owner/repo",
				"/tmp/brief-checkout",
				Options{Depth: tt.depth},
			)
			if err != nil {
				t.Fatalf("cloneInto: %v", err)
			}
			if !called {
				t.Error("clone Ensure was not called")
			}
		})
	}
}

func TestHTTPSCloneCache(t *testing.T) {
	originalEnsure := ensureClone
	originalCache := prepareCloneCache
	t.Cleanup(func() {
		ensureClone = originalEnsure
		prepareCloneCache = originalCache
	})

	ensureClone = func(context.Context, string, string, bool) error {
		return errors.New("unexpected clone Ensure")
	}
	called := false
	prepareCloneCache = func(_ context.Context, root, url, dst string) error {
		called = true
		if root != "/tmp/brief-cache" {
			t.Errorf("cache root = %q", root)
		}
		if url != "https://example.com/owner/repo" {
			t.Errorf("url = %q", url)
		}
		if dst != "/tmp/brief-checkout" {
			t.Errorf("dst = %q", dst)
		}
		return nil
	}

	err := cloneInto(
		context.Background(),
		"https://example.com/owner/repo",
		"/tmp/brief-checkout",
		Options{Cache: "/tmp/brief-cache"},
	)
	if err != nil {
		t.Fatalf("cloneInto: %v", err)
	}
	if !called {
		t.Error("clone cache was not prepared")
	}
}

func TestHTTPSCloneCacheError(t *testing.T) {
	originalCache := prepareCloneCache
	t.Cleanup(func() { prepareCloneCache = originalCache })

	prepareCloneCache = func(context.Context, string, string, string) error {
		return errors.New("cache failed")
	}
	err := cloneInto(
		context.Background(),
		"https://example.com/owner/repo",
		"/tmp/brief-checkout",
		Options{Cache: "/tmp/brief-cache"},
	)
	if err == nil || !strings.Contains(err.Error(), "preparing clone cache: cache failed") {
		t.Fatalf("cloneInto error = %v", err)
	}
}

func TestNonHTTPSCloneUsesGitCommand(t *testing.T) {
	originalEnsure := ensureClone
	originalExec := execClone
	t.Cleanup(func() {
		ensureClone = originalEnsure
		execClone = originalExec
	})

	ensureClone = func(context.Context, string, string, bool) error {
		return errors.New("unexpected clone Ensure")
	}
	called := false
	execClone = func(_ context.Context, url, dst string, depth int) error {
		called = true
		if url != "git@example.com:owner/repo.git" {
			t.Errorf("url = %q", url)
		}
		if dst != "/tmp/brief-checkout" {
			t.Errorf("dst = %q", dst)
		}
		if depth != 7 {
			t.Errorf("depth = %d", depth)
		}
		return nil
	}

	err := cloneInto(
		context.Background(),
		"git@example.com:owner/repo.git",
		"/tmp/brief-checkout",
		Options{Depth: 7},
	)
	if err != nil {
		t.Fatalf("cloneInto: %v", err)
	}
	if !called {
		t.Error("Git clone command was not called")
	}
}
