package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

func TestDirectoryExpand(t *testing.T) {
	dir := t.TempDir()
	files := []string{"check_one", "check_two", "not_a_check"}
	for _, name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	dcfg := DirectoryConfig{
		NamePrefix: "auto_",
		Path:       dir,
		Include:    []string{"check_*"},
		Exclude:    []string{"*two"},
		ArgValues:  []string{"8080"},
		Defaults: Defaults{
			Timeout:       confopt.Duration(30 * time.Second),
			RetryInterval: confopt.Duration(30 * time.Second),
		},
	}

	jobs, err := dcfg.Expand(Defaults{})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "auto_check_one" {
		t.Fatalf("unexpected job name %s", jobs[0].Name)
	}
	if jobs[0].Plugin == "" {
		t.Fatalf("plugin path must be set")
	}
	if len(jobs[0].ArgValues) != 1 || jobs[0].ArgValues[0] != "8080" {
		t.Fatalf("arg_values not propagated")
	}
}
