// SPDX-License-Identifier: GPL-3.0-or-later

package promtestdata

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUsesConfiguredTestdataRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prometheus", "fixture.prom")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# TYPE value gauge\nvalue 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(DirEnv, root)

	got, err := resolve("prometheus/fixture.prom")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("resolved path: got %q, want %q", got, path)
	}
}

func TestResolveRejectsPathsOutsideTestdataRoot(t *testing.T) {
	t.Setenv(DirEnv, t.TempDir())

	for _, path := range []string{"", ".", "..", "../fixture.prom", "/fixture.prom"} {
		t.Run(path, func(t *testing.T) {
			_, err := resolve(path)
			if !errors.Is(err, errInvalidPath) {
				t.Fatalf("resolve(%q) error: got %v, want invalid-path error", path, err)
			}
		})
	}
}

func TestResolveRejectsMissingAndNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv(DirEnv, root)

	if _, err := resolve("missing.prom"); err == nil {
		t.Fatal("missing file resolved without an error")
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve("directory"); err == nil {
		t.Fatal("directory resolved as a regular file")
	}
}
