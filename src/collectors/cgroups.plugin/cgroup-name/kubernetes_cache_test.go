// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustNewKubernetesCache(t *testing.T, dir string) kubernetesCache {
	t.Helper()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod cache directory %q: %v", dir, err)
	}
	cache, err := newKubernetesCache(dir)
	if err != nil {
		t.Fatalf("newKubernetesCache(%q): %v", dir, err)
	}
	return cache
}

func TestNewKubernetesCacheRequiresPrivateDirectory(t *testing.T) {
	base := t.TempDir()
	privateDir := filepath.Join(base, "private")
	if err := os.Mkdir(privateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	publicDir := filepath.Join(base, "public")
	if err := os.Mkdir(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkDir := filepath.Join(base, "symlink")
	if err := os.Symlink(privateDir, symlinkDir); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(base, "regular-file")
	if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		dir     string
		wantErr bool
	}{
		"missing directory is created privately": {
			dir: filepath.Join(base, "created"),
		},
		"existing private directory": {
			dir: privateDir,
		},
		"public directory is rejected": {
			dir:     publicDir,
			wantErr: true,
		},
		"symlink directory is rejected": {
			dir:     symlinkDir,
			wantErr: true,
		},
		"empty directory is rejected": {
			wantErr: true,
		},
		"missing parent is rejected": {
			dir:     filepath.Join(base, "missing", "child"),
			wantErr: true,
		},
		"regular file is rejected": {
			dir:     filePath,
			wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newKubernetesCache(test.dir)
			if got := err != nil; got != test.wantErr {
				t.Fatalf("newKubernetesCache(%q) error = %v, wantErr=%v", test.dir, err, test.wantErr)
			}
			if err == nil {
				info, statErr := os.Lstat(test.dir)
				if statErr != nil {
					t.Fatal(statErr)
				}
				if info.Mode().Perm() != 0o700 {
					t.Fatalf("directory mode = %o, want 0700", info.Mode().Perm())
				}
			}
		})
	}
}

func TestKubernetesCacheMetadataWritesAreBounded(t *testing.T) {
	cache := mustNewKubernetesCache(t, t.TempDir())
	tests := map[string]struct {
		write   func() error
		wantErr bool
	}{
		"empty cluster name is a no-op": {
			write: func() error { return cache.writeClusterName("") },
		},
		"oversized cluster name is rejected": {
			write:   func() error { return cache.writeClusterName(strings.Repeat("x", maxKubernetesCacheMetadataLine)) },
			wantErr: true,
		},
		"empty system UID is a no-op": {
			write: func() error { return cache.writeSystemUID("") },
		},
		"oversized system UID is rejected": {
			write:   func() error { return cache.writeSystemUID(strings.Repeat("x", maxKubernetesCacheMetadataLine)) },
			wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := test.write(); (err != nil) != test.wantErr {
				t.Fatalf("write error = %v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestWritePrivateFileIsSymlinkSafe(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
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

func TestWritePrivateFileRejectsUnsafeDestinations(t *testing.T) {
	privateDir := t.TempDir()
	if err := os.Chmod(privateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	publicDir := t.TempDir()
	if err := os.Chmod(publicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	directoryDestination := filepath.Join(privateDir, "directory")
	if err := os.Mkdir(directoryDestination, 0o700); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		path string
	}{
		"empty path": {},
		"missing parent": {
			path: filepath.Join(privateDir, "missing", "cache"),
		},
		"public parent": {
			path: filepath.Join(publicDir, "cache"),
		},
		"directory destination": {
			path: directoryDestination,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if err := writePrivateFile(test.path, []byte("fixture")); err == nil {
				t.Fatal("unsafe write unexpectedly succeeded")
			}
		})
	}
}

func TestCacheReadHelpersRejectUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(`container_id="leaked"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cache-link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	public := filepath.Join(dir, "public")
	if err := os.WriteFile(public, []byte(`container_id="leaked"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hardlinkSource := filepath.Join(dir, "hardlink-source")
	if err := os.WriteFile(hardlinkSource, []byte(`container_id="leaked"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hardlink := filepath.Join(dir, "hardlink")
	if err := os.Link(hardlinkSource, hardlink); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		path        string
		wantRegular bool
		wantFirst   string
		wantLookup  bool
		wantOpen    bool
	}{
		"regular file": {
			path:        regular,
			wantRegular: true,
			wantFirst:   `container_id="leaked"`,
			wantLookup:  true,
			wantOpen:    true,
		},
		"symlink": {
			path: link,
		},
		"public permissions": {
			path: public,
		},
		"multiple hard links": {
			path: hardlink,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isPrivateRegularFile(test.path, 1<<20); got != test.wantRegular {
				t.Fatalf("isPrivateRegularFile = %v, want %v", got, test.wantRegular)
			}
			if got := firstLineFile(test.path, 1<<20); got != test.wantFirst {
				t.Fatalf("firstLineFile = %q, want %q", got, test.wantFirst)
			}
			if _, got := findContainerLabelSetInFile(context.Background(), test.path, "leaked"); got != test.wantLookup {
				t.Fatalf("field lookup matched = %v, want %v", got, test.wantLookup)
			}
			file, err := openPrivateRegularFile(test.path, 1<<20)
			if file != nil {
				_ = file.Close()
			}
			if got := err == nil; got != test.wantOpen {
				t.Fatalf("openPrivateRegularFile success = %v, want %v (error: %v)", got, test.wantOpen, err)
			}
		})
	}
	if file, err := openPrivateRegularFile(regular, 4); err == nil || file != nil {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("oversized cache file was accepted: %v", err)
	}
}

func TestCacheLookupDoesNotParseEveryPrecedingRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netdata-cgroups-containers")
	const target = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	var contents strings.Builder
	for index := range 2048 {
		fmt.Fprintf(&contents, `namespace="default",pod_name="pod-%d",pod_uid="%064x",node_name="node",container_name="app",container_id="%064x"`+"\n", index, index, index)
	}
	fmt.Fprintf(&contents, `namespace="default",pod_name="target",pod_uid="%064x",node_name="node",container_name="app",container_id="%s"`+"\n", 2048, target)
	if err := os.WriteFile(path, []byte(contents.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			if _, ok := findContainerLabelSetInFile(context.Background(), path, target); !ok {
				b.Fatal("cache match not found")
			}
		}
	})
	if got, max := result.AllocedBytesPerOp(), int64(512<<10); got > max {
		t.Fatalf("cache lookup allocated %d bytes/op, want at most %d", got, max)
	}
}

func TestCacheLookupHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netdata-cgroups-containers")
	if err := os.WriteFile(path, []byte(`container_id="needle"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := findContainerLabelSetInFile(ctx, path, "needle"); ok {
		t.Fatal("cancelled cache lookup unexpectedly matched")
	}
}
