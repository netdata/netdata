// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePrivateFileIsSymlinkSafe(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("original-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "netdata-cgroups-containers")
	if err := os.Symlink(victim, destination); err != nil {
		t.Fatal(err)
	}

	if err := writePrivateFile(destination, []byte("cache-payload\n")); err != nil {
		t.Fatalf("writePrivateFile: %v", err)
	}
	if got, _ := os.ReadFile(victim); string(got) != "original-secret\n" {
		t.Fatalf("victim was clobbered through the symlink: %q", got)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("destination must be a regular file, got mode %v", info.Mode())
	}
	if permission := info.Mode().Perm(); permission != 0o600 {
		t.Fatalf("destination mode = %v, want 0600", permission)
	}
	if got, _ := os.ReadFile(destination); string(got) != "cache-payload\n" {
		t.Fatalf("destination content = %q", got)
	}
}

func TestCacheReadHelpersRejectSymlinks(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("leaked-line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cache-link")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}

	if isPrivateRegularFile(link) {
		t.Fatal("isPrivateRegularFile must reject a symlink")
	}
	if !isPrivateRegularFile(secret) {
		t.Fatal("isPrivateRegularFile must accept a regular file")
	}
	if got := firstLineFile(link); got != "" {
		t.Fatalf("firstLineFile followed a symlink: %q", got)
	}
	if _, ok := grepFile(link, "leaked", 0); ok {
		t.Fatal("grepFile followed a symlink")
	}
	if got := firstLineFile(secret); got != "leaked-line" {
		t.Fatalf("firstLineFile on a regular file = %q", got)
	}
}

func TestCacheLookupDoesNotAllocateTheWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netdata-cgroups-containers")
	contents := `container_id="needle",pod_name="first"` + "\n" + strings.Repeat("x", 4<<20)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			if _, ok := grepFile(path, "needle", 1); !ok {
				b.Fatal("cache match not found")
			}
		}
	})
	if got, max := result.AllocedBytesPerOp(), int64(512<<10); got > max {
		t.Fatalf("cache lookup allocated %d bytes/op, want at most %d", got, max)
	}
}
