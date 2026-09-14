//go:build linux

package posix

import (
	"fmt"
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
//  ShmContext accessor methods
// ---------------------------------------------------------------------------

func TestShmContextRole(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_role"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer ctx.ShmDestroy()

	if ctx.Role() != ShmRoleServer {
		t.Errorf("expected ShmRoleServer, got %d", ctx.Role())
	}
	if ctx.Fd() < 0 {
		t.Errorf("expected fd >= 0, got %d", ctx.Fd())
	}
}

func TestShmContextClientRole(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_crole"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	srv, err := ShmServerCreate(testShmRunDir, svc, 10, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer srv.ShmDestroy()

	client, err := ShmClientAttach(testShmRunDir, svc, 10)
	if err != nil {
		t.Fatalf("client attach: %v", err)
	}
	defer client.ShmClose()

	if client.Role() != ShmRoleClient {
		t.Errorf("expected ShmRoleClient, got %d", client.Role())
	}
	if client.Fd() < 0 {
		t.Errorf("expected fd >= 0, got %d", client.Fd())
	}
}

// ---------------------------------------------------------------------------
//  OwnerAlive
// ---------------------------------------------------------------------------

func TestShmOwnerAlive(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_alive"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer ctx.ShmDestroy()

	// Owner is current process, should be alive
	if !ctx.OwnerAlive() {
		t.Error("expected OwnerAlive() = true for current process")
	}
}

// ---------------------------------------------------------------------------
//  validateShmServiceName edge cases
// ---------------------------------------------------------------------------

func TestValidateShmServiceNameEdgeCases(t *testing.T) {
	valid := []string{"a", "test-svc", "test.svc", "test_svc", "Test123"}
	for _, name := range valid {
		if err := validateShmServiceName(name); err != nil {
			t.Errorf("validateShmServiceName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", ".", "..", "a/b", "a b", "a@b", "a#b"}
	for _, name := range invalid {
		if err := validateShmServiceName(name); err == nil {
			t.Errorf("validateShmServiceName(%q) = nil, want error", name)
		}
	}
}

// ---------------------------------------------------------------------------
//  buildShmPath edge cases
// ---------------------------------------------------------------------------

func TestBuildShmPathEdgeCases(t *testing.T) {
	// Valid
	path, err := buildShmPath("/tmp", "test", 1)
	if err != nil {
		t.Fatalf("buildShmPath: %v", err)
	}
	expected := fmt.Sprintf("/tmp/test-%016x.ipcshm", uint64(1))
	if path != expected {
		t.Fatalf("path = %q, want %q", path, expected)
	}

	// Invalid service name
	_, err = buildShmPath("/tmp", "", 1)
	if err == nil {
		t.Fatal("expected error for empty service name")
	}

	// Path too long
	longDir := "/tmp/" + string(make([]byte, 300))
	_, err = buildShmPath(longDir, "test", 1)
	if err != ErrShmPathTooLong {
		t.Fatalf("expected ErrShmPathTooLong, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  pidAlive edge cases
// ---------------------------------------------------------------------------

func TestPidAliveEdgeCases(t *testing.T) {
	// PID 0 or negative should return false
	if pidAlive(0) {
		t.Error("pidAlive(0) should be false")
	}
	if pidAlive(-1) {
		t.Error("pidAlive(-1) should be false")
	}

	// Current process should be alive
	if !pidAlive(os.Getpid()) {
		t.Error("current process should be alive")
	}
}

// ---------------------------------------------------------------------------
//  Atomic operation bounds checking
// ---------------------------------------------------------------------------

func TestAtomicLoadU64OutOfBounds(t *testing.T) {
	data := make([]byte, 4) // too small for u64
	_, err := atomicLoadU64(data, 0)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds, got %v", err)
	}

	_, err = atomicLoadU64(data, -1)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds for negative offset, got %v", err)
	}
}

func TestAtomicLoadU32OutOfBounds(t *testing.T) {
	data := make([]byte, 2) // too small for u32
	_, err := atomicLoadU32(data, 0)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds, got %v", err)
	}
}

func TestAtomicStoreU32OutOfBounds(t *testing.T) {
	data := make([]byte, 2)
	err := atomicStoreU32(data, 0, 42)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds, got %v", err)
	}
}

func TestAtomicAddU64OutOfBounds(t *testing.T) {
	data := make([]byte, 4)
	err := atomicAddU64(data, 0, 1)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds, got %v", err)
	}
}

func TestAtomicAddU32OutOfBounds(t *testing.T) {
	data := make([]byte, 2)
	err := atomicAddU32(data, 0, 1)
	if err != errShmOutOfBounds {
		t.Fatalf("expected errShmOutOfBounds, got %v", err)
	}
}

func TestFutexWakeCallOutOfBounds(t *testing.T) {
	data := make([]byte, 2)
	ret := futexWakeCall(data, 0, 1)
	if ret != -1 {
		t.Fatalf("expected -1 for out-of-bounds, got %d", ret)
	}
}

func TestFutexWaitCallOutOfBounds(t *testing.T) {
	data := make([]byte, 2)
	ret := futexWaitCall(data, 0, 0, nil)
	if ret != -1 {
		t.Fatalf("expected -1 for out-of-bounds, got %d", ret)
	}
}

// ---------------------------------------------------------------------------
//  SHM client attach: file does not exist
// ---------------------------------------------------------------------------

func TestShmClientAttachMissing(t *testing.T) {
	ensureShmRunDir(t)
	_, err := ShmClientAttach(testShmRunDir, "nonexistent", 99999)
	if err == nil {
		t.Fatal("expected error for non-existent SHM file")
	}
}

// ---------------------------------------------------------------------------
//  SHM close/destroy double-call safety
// ---------------------------------------------------------------------------

func TestShmCloseDoubleCall(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_dclose"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}

	// Double close should not panic
	ctx.ShmClose()
	ctx.ShmClose()
}

func TestShmDestroyDoubleCall(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_ddestroy"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 1024, 1024)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}

	// Double destroy should not panic
	ctx.ShmDestroy()
	ctx.ShmDestroy()
}

// ---------------------------------------------------------------------------
//  SHM send: message exceeds capacity
// ---------------------------------------------------------------------------

func TestShmSendTooLarge(t *testing.T) {
	ensureShmRunDir(t)
	svc := "go_shm_toolarge"
	cleanupShmFiles(t, svc)
	defer cleanupShmFiles(t, svc)

	ctx, err := ShmServerCreate(testShmRunDir, svc, 1, 64, 64)
	if err != nil {
		t.Fatalf("server create: %v", err)
	}
	defer ctx.ShmDestroy()

	// Try to send a message larger than the response capacity
	bigMsg := make([]byte, 200) // > 64 bytes
	err = ctx.ShmSend(bigMsg)
	if err == nil {
		t.Fatal("expected error for oversized message")
	}
}

// ---------------------------------------------------------------------------
//  SHM cleanup stale: no files to clean
// ---------------------------------------------------------------------------

func TestShmCleanupStaleNoFiles(t *testing.T) {
	ensureShmRunDir(t)
	// Should not panic even with no matching files
	ShmCleanupStale(testShmRunDir, "nonexistent_svc_xyz")
}
