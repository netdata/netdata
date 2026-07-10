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

	tests := map[string]struct {
		path        string
		wantRegular bool
		wantFirst   string
		wantGrep    bool
		wantOpen    bool
	}{
		"regular file": {
			path:        secret,
			wantRegular: true,
			wantFirst:   "leaked-line",
			wantGrep:    true,
			wantOpen:    true,
		},
		"symlink": {
			path: link,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isPrivateRegularFile(test.path); got != test.wantRegular {
				t.Fatalf("isPrivateRegularFile = %v, want %v", got, test.wantRegular)
			}
			if got := firstLineFile(test.path); got != test.wantFirst {
				t.Fatalf("firstLineFile = %q, want %q", got, test.wantFirst)
			}
			if _, got := grepFile(test.path, "leaked", 0); got != test.wantGrep {
				t.Fatalf("grepFile matched = %v, want %v", got, test.wantGrep)
			}
			file, err := openPrivateRegularFile(test.path)
			if file != nil {
				_ = file.Close()
			}
			if got := err == nil; got != test.wantOpen {
				t.Fatalf("openPrivateRegularFile success = %v, want %v (error: %v)", got, test.wantOpen, err)
			}
		})
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
