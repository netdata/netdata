//go:build !windows

package jobmgrtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolverDriverContainsCommandProcessGroup(t *testing.T) {
	directory := t.TempDir()
	helper := filepath.Join(directory, "resolver-helper")
	pidFile := filepath.Join(directory, "pids")
	script := "#!/bin/sh\n" +
		"sleep 30 &\n" +
		"child=$!\n" +
		"printf '%s %s\\n' \"$$\" \"$child\" > \"$1\"\n" +
		"wait \"$child\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()
	driver := ResolverDriver{Helper: helper, PIDFile: pidFile}
	if err := driver.Run(ctx, "F10.6"); err != nil {
		t.Fatal(err)
	}
}
