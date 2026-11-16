//go:build linux || (freebsd && (amd64 || 386))

package posix

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

const shmUmaskHelperEnv = "NETIPC_SHM_UMASK_HELPER"

// The umask is process-wide, so the restrictive-umask case runs in a child
// test process instead of the parallel test runner.
func TestShmAttachUnderRestrictiveUmask(t *testing.T) {
	ensureShmRunDir(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestShmUmaskHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), shmUmaskHelperEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("attach under umask 0477 failed: %v\n%s", err, out)
	}
}

// TestShmUmaskHelperProcess is the child entry point; it is inert unless
// spawned by TestShmAttachUnderRestrictiveUmask.
func TestShmUmaskHelperProcess(t *testing.T) {
	if os.Getenv(shmUmaskHelperEnv) == "" {
		return
	}
	// Restore the umask before exit so files the process writes then
	// (coverage data) keep their normal mode.
	defer syscall.Umask(syscall.Umask(0o477))
	svc := "go_shm_umask"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	server, err := ShmServerCreate(testShmRunDir, svc, 7101, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer server.ShmDestroy()
	path, err := buildShmPath(testShmRunDir, svc, 7101)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("region stat: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Fatalf("region file mode %o, want 600", mode)
	}
	client, err := ShmClientAttach(testShmRunDir, svc, 7101)
	if err != nil {
		t.Fatalf("client attach: %v", err)
	}
	client.ShmClose()
}
