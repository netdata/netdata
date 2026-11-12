//go:build unix

// SHM transport types shared by every POSIX system. The data plane needs a
// futex-style wait (shm_posix.go: Linux, and FreeBSD amd64 and 386); other
// systems get stubs (shm_other.go).

package posix

import "errors"

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrShmPathTooLong = errors.New("SHM path exceeds limit")
	ErrShmOpen        = errors.New("SHM open failed")
	ErrShmAllocate    = errors.New("SHM allocation failed")
	// Deprecated: SHM creation no longer uses ftruncate; use ErrShmAllocate.
	ErrShmTruncate    = errors.New("SHM ftruncate failed")
	ErrShmMmap        = errors.New("SHM mmap failed")
	ErrShmBadMagic    = errors.New("SHM header magic mismatch")
	ErrShmBadVersion  = errors.New("SHM header version mismatch")
	ErrShmBadHeader   = errors.New("SHM header_len mismatch")
	ErrShmBadSize     = errors.New("SHM file too small for declared areas")
	ErrShmAddrInUse   = errors.New("SHM region owned by live server")
	ErrShmNotReady    = errors.New("SHM server not ready")
	ErrShmMsgTooLarge = errors.New("message exceeds SHM area capacity")
	ErrShmTimeout     = errors.New("SHM futex wait timed out")
	ErrShmBadParam    = errors.New("invalid SHM argument")
	ErrShmPeerDead    = errors.New("SHM owner process has exited")
	// ErrShmUnsupported is returned on POSIX systems without SHM support.
	ErrShmUnsupported = errors.New("SHM transport not supported on this platform")
)

// ---------------------------------------------------------------------------
//  Role
// ---------------------------------------------------------------------------

// ShmRole distinguishes server vs client SHM contexts.
type ShmRole int

const (
	ShmRoleServer ShmRole = 1
	ShmRoleClient ShmRole = 2
)
