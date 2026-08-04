// SPDX-License-Identifier: GPL-3.0-or-later

// Package promtestdata resolves Prometheus profile evidence from the external
// netdata/testdata checkout used by tests.
package promtestdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// DirEnv overrides the default src/go/testdata checkout location.
	DirEnv = "NETDATA_TESTDATA_DIR"
	// RequiredEnv makes missing external Prometheus evidence a test failure.
	RequiredEnv = "NETDATA_PROMETHEUS_TESTDATA_REQUIRED"
)

var errInvalidPath = errors.New("invalid testdata path")

// Require returns the path to an external testdata file. Missing evidence
// skips the calling test locally and fails it when RequiredEnv is set to 1.
func Require(t testing.TB, relativePath string) string {
	t.Helper()

	path, err := resolve(relativePath)
	if err == nil {
		return path
	}
	if errors.Is(err, errInvalidPath) {
		t.Fatalf("invalid Prometheus testdata path %q: %v", relativePath, err)
	}

	message := fmt.Sprintf(
		"Prometheus profile testdata %q is unavailable: %v; clone https://github.com/netdata/testdata.git to src/go/testdata or set %s",
		relativePath,
		err,
		DirEnv,
	)
	if os.Getenv(RequiredEnv) == "1" {
		t.Fatal(message)
	}
	t.Skip(message)
	return ""
}

func resolve(relativePath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: must be a non-empty relative path below the testdata root", errInvalidPath)
	}

	root, err := testdataRoot()
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	return path, nil
}

func testdataRoot() (string, error) {
	if root := os.Getenv(DirEnv); root != "" {
		return filepath.Abs(root)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && info.Mode().IsRegular() {
			return filepath.Join(dir, "testdata"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate src/go/go.mod from the test working directory")
		}
		dir = parent
	}
}
