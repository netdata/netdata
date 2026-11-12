//go:build unix

package posix

import (
	"errors"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

type staleResult int

const (
	staleNotExist   staleResult = 0
	staleRecovered  staleResult = 1
	staleLiveServer staleResult = 2
)

const staleDialTimeout = 1 * time.Second

// dialStaleCandidate reports whether a live server accepts on path. The
// endpoint lock, not repeated dials, keeps a starting server's endpoint
// safe (see acquireEndpointLock).
func dialStaleCandidate(path string) error {
	conn, err := net.DialTimeout(staleDialNetwork, path, staleDialTimeout)
	if err == nil {
		_ = conn.Close()
	}
	return err
}

const lockOpenFlags = syscall.O_CLOEXEC | syscall.O_NOFOLLOW

var (
	lockPublishSeq atomic.Uint64
	// lockPublishHook, when set by tests, runs just before a new endpoint
	// lock file is linked to its final name.
	lockPublishHook atomic.Pointer[func(tmp string)]
)

// publishEndpointLock gives a missing lock its name only once it is 0600: a
// file created under a umask without owner bits stays unopenable if the
// process dies before Fchmod. Best effort: the caller's creating open covers
// every failure, such as a filesystem without hard links, and a losing Link
// means another starter published the file first.
func publishEndpointLock(lockPath string) {
	tmp := lockPath + "." + strconv.Itoa(os.Getpid()) + "." +
		strconv.FormatUint(lockPublishSeq.Add(1), 10) + ".tmp"
	fd, err := syscall.Open(tmp, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|lockOpenFlags, 0o600)
	if err != nil {
		return
	}
	err = syscall.Fchmod(fd, 0o600)
	_ = syscall.Close(fd)
	if err == nil {
		if hook := lockPublishHook.Load(); hook != nil {
			(*hook)(tmp)
		}
		_ = syscall.Link(tmp, lockPath)
	}
	_ = syscall.Unlink(tmp)
}

// acquireEndpointLock takes the endpoint lock {runDir}/{serviceName}.lock.
// A server's socket file exists from bind() until listen(), and connects in
// that window are refused just as for a dead server's leftover file, so a
// connect probe alone cannot tell a starting server from a stale one. The
// lock can: its holder owns the endpoint, and the kernel drops it when that
// process dies. The file stays on disk; removing it would let a starter
// that already opened it lock an orphaned inode.
func acquireEndpointLock(runDir, serviceName string) (*os.File, error) {
	lockPath := runDir + "/" + serviceName + ".lock"
	// A file left with mode bits missing, by an older build or a fallback
	// create under a umask without owner read or write, may not reopen
	// read-write, so a narrower open follows (flock works on any access mode)
	// and Fchmod restores 0600 for the next starter. Fchmod fails only for a
	// file another account owns, which the lock still works on.
	fd, err := syscall.Open(lockPath, syscall.O_RDWR|lockOpenFlags, 0)
	if errors.Is(err, syscall.ENOENT) {
		publishEndpointLock(lockPath)
		fd, err = syscall.Open(lockPath, syscall.O_RDWR|syscall.O_CREAT|lockOpenFlags, 0o600)
	}
	if errors.Is(err, syscall.EACCES) {
		fd, err = syscall.Open(lockPath, syscall.O_WRONLY|lockOpenFlags, 0)
	}
	if errors.Is(err, syscall.EACCES) {
		fd, err = syscall.Open(lockPath, syscall.O_RDONLY|lockOpenFlags, 0)
	}
	if err != nil {
		return nil, wrapErr(ErrSocket, "lock open: "+err.Error())
	}
	_ = syscall.Fchmod(fd, 0o600)
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		_ = syscall.Close(fd)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAddrInUse
		}
		return nil, wrapErr(ErrSocket, "lock: "+err.Error())
	}
	return os.NewFile(uintptr(fd), lockPath), nil
}

// releaseEndpointLock unlocks before closing: a child forked by another
// thread shares the open file description until it execs, and its copy
// would keep the lock held.
func releaseEndpointLock(lock *os.File) {
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	_ = lock.Close()
}

func checkAndRecoverStale(path string) staleResult {
	_, err := os.Stat(path)
	if err != nil {
		return staleNotExist
	}

	err = dialStaleCandidate(path)
	if err == nil {
		return staleLiveServer
	}

	if errors.Is(err, syscall.ENOENT) {
		return staleNotExist
	}
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
		// Cannot probe liveness (fd exhaustion) — keep the endpoint rather
		// than risk deleting a live socket.
		return staleLiveServer
	}
	// Nothing accepted the connection: a dead server's socket, or a foreign
	// file squatting on the endpoint path. Reclaim it (os.Remove also
	// handles a directory at the path).
	if removeErr := os.Remove(path); removeErr != nil {
		if os.IsNotExist(removeErr) {
			return staleNotExist
		}
		return staleLiveServer
	}
	return staleRecovered
}
