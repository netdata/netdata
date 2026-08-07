// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRunRequiresCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run(nil, &stdout, &stderr); got != 2 {
		t.Fatalf("exit code: got %d, want 2", got)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr does not contain usage: %q", stderr.String())
	}
}

func TestRunListsDiscoveredEvidenceDirectories(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if got := run([]string{"evidence-dirs", "--repo-root", repoRoot}, &stdout, &stderr); got != 0 {
		t.Fatalf("exit code: got %d, want 0\nstderr:\n%s", got, stderr.String())
	}
	directories := strings.Fields(stdout.String())
	if len(directories) == 0 {
		t.Fatal("no evidence directories listed")
	}
	if !slices.IsSorted(directories) {
		t.Fatalf("evidence directories are not sorted: %v", directories)
	}
	for _, directory := range directories {
		if !strings.HasPrefix(directory, "prometheus/profiles/") {
			t.Fatalf("unexpected evidence directory %q", directory)
		}
	}
}
