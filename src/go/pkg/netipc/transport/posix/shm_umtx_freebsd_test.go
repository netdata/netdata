//go:build freebsd && (amd64 || 386)

package posix

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The raw posix_fallocate syscall reports its error in the return value;
// the per-architecture argument layout must reserve exactly the range.
func TestPosixFallocateRawSyscall(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "region"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Above 4 GiB, so both halves of each 64-bit argument matter on 386.
	const off, length = int64(5) << 30, int64(8192)
	if errno := posixFallocate(int(f.Fd()), off, length); errno != 0 {
		t.Fatalf("posix_fallocate on a file: %v", errno)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != off+length {
		t.Fatalf("size %d, want %d", info.Size(), off+length)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if errno := posixFallocate(int(w.Fd()), 0, 4096); errno != syscall.ESPIPE && errno != syscall.ENODEV {
		t.Fatalf("posix_fallocate on a pipe: %v, want ESPIPE or ENODEV", errno)
	}
}

// A wait with a stale expected value returns at once; a wait on the
// current value times out with ETIMEDOUT.
func TestUmtxWaitStaleValueAndTimeout(t *testing.T) {
	word := make([]byte, 8)
	if ret := futexWaitCall(word, 0, 1, nil); ret != 0 {
		t.Fatalf("stale wait = %d, want 0", ret)
	}
	ts := syscall.NsecToTimespec(10_000_000)
	if ret := futexWaitCall(word, 0, 0, &ts); ret != -int(syscall.ETIMEDOUT) {
		t.Fatalf("timed wait = %d, want -ETIMEDOUT", ret)
	}
	if ret := futexWakeCall(word, 0, 1); ret < 0 {
		t.Fatalf("wake = %d", ret)
	}
}
