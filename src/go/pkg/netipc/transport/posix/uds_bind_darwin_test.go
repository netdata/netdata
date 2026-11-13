//go:build darwin

package posix

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// macOS binds in {service}.d/ and renames the socket into place.
func TestListenBindDir(t *testing.T) {
	runDir := testRunDir(t)
	service := uniqueService(t)
	dir := filepath.Join(runDir, service+".d")
	path := filepath.Join(runDir, service+".sock")
	t.Cleanup(func() { _ = os.Remove(path) })
	gone := func(p string) bool {
		_, err := os.Lstat(p)
		return errors.Is(err, os.ErrNotExist)
	}

	listener := startListener(t, runDir, service, defaultServerConfig())
	// Close is idempotent: an early failure below still releases the endpoint.
	t.Cleanup(listener.Close)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("socket mode %o, want 0600", mode)
	}
	if !gone(dir) {
		t.Fatal("bind directory not removed")
	}
	listener.Close()

	// A crashed server's leftover directory is reclaimed.
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	startListener(t, runDir, service, defaultServerConfig()).Close()
	if !gone(dir) {
		t.Fatal("leftover directory not removed")
	}

	// A non-directory at the name is junk, as at the socket name.
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	startListener(t, runDir, service, defaultServerConfig()).Close()
	if !gone(dir) {
		t.Fatal("leftover file not removed")
	}

	// Foreign content is never deleted: listen fails instead.
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(other); _ = os.Remove(dir) })
	if l, err := Listen(runDir, service, defaultServerConfig()); !errors.Is(err, ErrSocket) {
		if l != nil {
			l.Close()
		}
		t.Fatalf("listen over foreign content: %v, want ErrSocket", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("foreign content removed: %v", err)
	}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}

	// A directory where the socket goes is foreign content too.
	sub := filepath.Join(dir, "s")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(sub) })
	if l, err := Listen(runDir, service, defaultServerConfig()); !errors.Is(err, ErrSocket) {
		if l != nil {
			l.Close()
		}
		t.Fatalf("listen over a directory at the socket name: %v, want ErrSocket", err)
	}
	if info, err := os.Stat(sub); err != nil || !info.IsDir() {
		t.Fatalf("foreign subdirectory removed: %v", err)
	}
}
