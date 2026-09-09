// SPDX-License-Identifier: GPL-3.0-or-later

package promtestutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const requireHelperEnv = "NETDATA_PROMTESTDATA_REQUIRE_HELPER"

func TestRequireHelperProcess(t *testing.T) {
	if os.Getenv(requireHelperEnv) != "1" {
		return
	}
	if path := Require(t, "prometheus/fixture.prom"); path == "" {
		t.Fatal("Require returned an empty path")
	}
}

func TestRequireSkipsOnlyWhenCheckoutIsAbsent(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	output, err := runRequireHelper(t, missingRoot, false)
	if err != nil {
		t.Fatalf("optional absent checkout failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "--- SKIP: TestRequireHelperProcess") {
		t.Fatalf("optional absent checkout did not skip:\n%s", output)
	}

	output, err = runRequireHelper(t, missingRoot, true)
	if err == nil {
		t.Fatalf("required absent checkout passed:\n%s", output)
	}

	presentRoot := t.TempDir()
	output, err = runRequireHelper(t, presentRoot, false)
	if err == nil {
		t.Fatalf("incomplete present checkout passed:\n%s", output)
	}
}

func TestRequireReturnsExistingEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prometheus", "fixture.prom")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# TYPE value gauge\nvalue 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := runRequireHelper(t, root, false)
	if err != nil {
		t.Fatalf("existing evidence failed: %v\n%s", err, output)
	}
}

func runRequireHelper(t *testing.T, root string, required bool) (string, error) {
	t.Helper()

	requiredValue := ""
	if required {
		requiredValue = "1"
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRequireHelperProcess$", "-test.v")
	command.Env = append(
		environmentWithout(DirEnv, RequiredEnv, requireHelperEnv),
		DirEnv+"="+root,
		RequiredEnv+"="+requiredValue,
		requireHelperEnv+"=1",
	)
	output, err := command.CombinedOutput()
	return string(output), err
}

func environmentWithout(keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key] = true
	}

	environment := os.Environ()
	filtered := environment[:0]
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if !blocked[key] {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

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

	if runtime.GOOS != "windows" {
		target := filepath.Join(root, "target.prom")
		if err := os.WriteFile(target, []byte("value 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "symlink.prom")); err != nil {
			t.Fatal(err)
		}
		if _, err := resolve("symlink.prom"); err == nil {
			t.Fatal("symlink resolved as a regular file")
		}
	}
}
