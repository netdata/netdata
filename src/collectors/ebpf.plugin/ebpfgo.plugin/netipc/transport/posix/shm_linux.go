//go:build linux

// SHM transport for Linux — shared memory data plane with spin+futex
// synchronization. Wire-compatible with the C and Rust implementations.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.

package posix

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const (
	shmRegionMagic     uint32 = 0x4e53484d // "NSHM"
	shmRegionVersion   uint16 = 3
	shmRegionAlignment uint32 = 64
	shmHeaderLen       uint16 = 64
	shmDefaultSpin     uint32 = 128

	// Byte offsets of all fields in the 64-byte region header.
	shmHeaderMagicOff      = 0
	shmHeaderVersionOff    = 4
	shmHeaderHeaderLenOff  = 6
	shmHeaderOwnerPidOff   = 8
	shmHeaderOwnerGenOff   = 12
	shmHeaderReqOffOff     = 16
	shmHeaderReqCapOff     = 20
	shmHeaderRespOffOff    = 24
	shmHeaderRespCapOff    = 28
	shmHeaderReqSeqOff     = 32
	shmHeaderRespSeqOff    = 40
	shmHeaderReqLenOff     = 48
	shmHeaderRespLenOff    = 52
	shmHeaderReqSignalOff  = 56
	shmHeaderRespSignalOff = 60

	// futex operations
	futexWait = 0
	futexWake = 1

	shmMaxPath = 256
)

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrShmPathTooLong = errors.New("SHM path exceeds limit")
	ErrShmOpen        = errors.New("SHM open failed")
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

// ---------------------------------------------------------------------------
//  SHM context
// ---------------------------------------------------------------------------

// ShmContext is a handle to a shared memory region.
type ShmContext struct {
	role ShmRole
	fd   int
	data []byte // mmap'd region (via syscall.Mmap)

	requestOffset    uint32
	requestCapacity  uint32
	responseOffset   uint32
	responseCapacity uint32

	localReqSeq  uint64
	localRespSeq uint64

	SpinTries       uint32
	ownerGeneration uint32 // cached for PID reuse detection
	path            string
}

// Role returns the context role.
func (c *ShmContext) Role() ShmRole { return c.role }

// Fd returns the file descriptor.
func (c *ShmContext) Fd() int { return c.fd }

// OwnerAlive checks if the region's owner process is still alive.
func (c *ShmContext) OwnerAlive() bool {
	if len(c.data) < int(shmHeaderLen) {
		return false
	}
	pid := int32(binary.NativeEndian.Uint32(c.data[shmHeaderOwnerPidOff : shmHeaderOwnerPidOff+4]))
	if !pidAlive(int(pid)) {
		return false
	}
	// Verify generation matches to detect PID reuse.
	// Skip check if cached generation is 0 (legacy region).
	if c.ownerGeneration != 0 {
		curGen := binary.NativeEndian.Uint32(c.data[shmHeaderOwnerGenOff : shmHeaderOwnerGenOff+4])
		if curGen != c.ownerGeneration {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
//  Server API
// ---------------------------------------------------------------------------

// ShmServerCreate creates a SHM region at {runDir}/{serviceName}-{sessionID}.ipcshm.
func ShmServerCreate(runDir, serviceName string, sessionID uint64, reqCapacity, respCapacity uint32) (*ShmContext, error) {
	path, err := buildShmPath(runDir, serviceName, sessionID)
	if err != nil {
		return nil, err
	}

	// Round capacities
	reqCap := shmAlign64(reqCapacity)
	respCap := shmAlign64(respCapacity)

	reqOff := shmAlign64(uint32(shmHeaderLen))
	respOff := shmAlign64(reqOff + reqCap)
	regionSize := int(respOff + respCap)

	// Try O_EXCL create first (fast path, no stale check needed).
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil && os.IsExist(err) {
		// File exists — do stale recovery and retry.
		stale := checkShmStale(path)
		if stale == shmStaleLive {
			return nil, fmt.Errorf("%w: live server owns SHM region", ErrShmOpen)
		}
		// Stale file was unlinked, retry create
		f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrShmOpen, err)
	}
	fd := int(f.Fd())

	if err := syscall.Ftruncate(fd, int64(regionSize)); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("%w: %v", ErrShmTruncate, err)
	}

	data, err := syscall.Mmap(fd, 0, regionSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("%w: %v", ErrShmMmap, err)
	}

	// Zero region (Mmap may return zero-filled from kernel, but be explicit)
	for i := range data {
		data[i] = 0
	}

	// Use a time-based generation to detect PID reuse across restarts.
	now := time.Now()
	generation := uint32(now.Unix()) ^ uint32(now.Nanosecond()>>10)

	// Write header fields (host byte order)
	binary.NativeEndian.PutUint32(data[shmHeaderMagicOff:shmHeaderMagicOff+4], shmRegionMagic)
	binary.NativeEndian.PutUint16(data[shmHeaderVersionOff:shmHeaderVersionOff+2], shmRegionVersion)
	binary.NativeEndian.PutUint16(data[shmHeaderHeaderLenOff:shmHeaderHeaderLenOff+2], uint16(shmHeaderLen))
	binary.NativeEndian.PutUint32(data[shmHeaderOwnerPidOff:shmHeaderOwnerPidOff+4], uint32(int32(os.Getpid())))
	binary.NativeEndian.PutUint32(data[shmHeaderOwnerGenOff:shmHeaderOwnerGenOff+4], generation)
	binary.NativeEndian.PutUint32(data[shmHeaderReqOffOff:shmHeaderReqOffOff+4], reqOff)
	binary.NativeEndian.PutUint32(data[shmHeaderReqCapOff:shmHeaderReqCapOff+4], reqCap)
	binary.NativeEndian.PutUint32(data[shmHeaderRespOffOff:shmHeaderRespOffOff+4], respOff)
	binary.NativeEndian.PutUint32(data[shmHeaderRespCapOff:shmHeaderRespCapOff+4], respCap)

	// Release fence: ensure header writes are visible before clients
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&data[shmHeaderReqSignalOff])), 0)

	// Close the os.File but keep the fd open (Mmap holds a reference).
	// Actually, we need to keep the fd ourselves for the context.
	// os.File.Close() would close the fd, so we dup first or just
	// not use os.File.Close(). We already have the fd from f.Fd().
	// Trick: prevent Go's finalizer from closing fd.
	// The safe way: dup the fd, then close the file.
	newFd, err := syscall.Dup(fd)
	if err != nil {
		syscall.Munmap(data)
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("%w: dup: %v", ErrShmOpen, err)
	}
	f.Close() // closes original fd

	return &ShmContext{
		role:             ShmRoleServer,
		fd:               newFd,
		data:             data,
		requestOffset:    reqOff,
		requestCapacity:  reqCap,
		responseOffset:   respOff,
		responseCapacity: respCap,
		localReqSeq:      0,
		localRespSeq:     0,
		SpinTries:        shmDefaultSpin,
		ownerGeneration:  generation,
		path:             path,
	}, nil
}

// ShmDestroy destroys a server SHM region (munmap, close, unlink).
func (c *ShmContext) ShmDestroy() {
	if c.data != nil {
		syscall.Munmap(c.data)
		c.data = nil
	}
	if c.fd >= 0 {
		syscall.Close(c.fd)
		c.fd = -1
	}
	if c.path != "" {
		os.Remove(c.path)
		c.path = ""
	}
}

// ---------------------------------------------------------------------------
//  Client API
// ---------------------------------------------------------------------------

// ShmClientAttach attaches to an existing SHM region.
func ShmClientAttach(runDir, serviceName string, sessionID uint64) (*ShmContext, error) {
	path, err := buildShmPath(runDir, serviceName, sessionID)
	if err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrShmOpen, err)
	}
	fd := int(f.Fd())

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: stat: %v", ErrShmOpen, err)
	}

	fileSize := int(info.Size())
	if fileSize < int(shmHeaderLen) {
		f.Close()
		return nil, ErrShmNotReady
	}

	data, err := syscall.Mmap(fd, 0, fileSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%w: %v", ErrShmMmap, err)
	}

	// Acquire fence
	atomic.LoadUint32((*uint32)(unsafe.Pointer(&data[shmHeaderReqSignalOff])))

	// Validate header
	magic := binary.NativeEndian.Uint32(data[shmHeaderMagicOff : shmHeaderMagicOff+4])
	if magic != shmRegionMagic {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmBadMagic
	}

	version := binary.NativeEndian.Uint16(data[shmHeaderVersionOff : shmHeaderVersionOff+2])
	if version != shmRegionVersion {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmBadVersion
	}

	hdrLen := binary.NativeEndian.Uint16(data[shmHeaderHeaderLenOff : shmHeaderHeaderLenOff+2])
	if hdrLen != uint16(shmHeaderLen) {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmBadHeader
	}

	reqOff := binary.NativeEndian.Uint32(data[shmHeaderReqOffOff : shmHeaderReqOffOff+4])
	reqCap := binary.NativeEndian.Uint32(data[shmHeaderReqCapOff : shmHeaderReqCapOff+4])
	respOff := binary.NativeEndian.Uint32(data[shmHeaderRespOffOff : shmHeaderRespOffOff+4])
	respCap := binary.NativeEndian.Uint32(data[shmHeaderRespCapOff : shmHeaderRespCapOff+4])

	headerEnd := shmAlign64(uint32(shmHeaderLen))
	if reqOff < headerEnd || reqCap == 0 || respOff < headerEnd || respCap == 0 {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmNotReady
	}

	if reqOff%shmRegionAlignment != 0 ||
		reqCap%shmRegionAlignment != 0 ||
		respOff%shmRegionAlignment != 0 ||
		respCap%shmRegionAlignment != 0 ||
		respOff < shmAlign64(reqOff+reqCap) {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmBadSize
	}

	// Validate region size
	reqEnd := int(reqOff) + int(reqCap)
	respEnd := int(respOff) + int(respCap)
	needed := max(respEnd, reqEnd)
	if fileSize < needed {
		syscall.Munmap(data)
		f.Close()
		return nil, ErrShmBadSize
	}

	// Read current sequence numbers
	curReqSeq, err := atomicLoadU64(data, shmHeaderReqSeqOff)
	if err != nil {
		syscall.Munmap(data)
		f.Close()
		return nil, fmt.Errorf("%w: load req_seq: %v", ErrShmBadParam, err)
	}
	curRespSeq, err := atomicLoadU64(data, shmHeaderRespSeqOff)
	if err != nil {
		syscall.Munmap(data)
		f.Close()
		return nil, fmt.Errorf("%w: load resp_seq: %v", ErrShmBadParam, err)
	}
	ownerGen := binary.NativeEndian.Uint32(data[shmHeaderOwnerGenOff : shmHeaderOwnerGenOff+4])

	// Dup fd and close file
	newFd, err := syscall.Dup(fd)
	if err != nil {
		syscall.Munmap(data)
		f.Close()
		return nil, fmt.Errorf("%w: dup: %v", ErrShmOpen, err)
	}
	f.Close()

	return &ShmContext{
		role:             ShmRoleClient,
		fd:               newFd,
		data:             data,
		requestOffset:    reqOff,
		requestCapacity:  reqCap,
		responseOffset:   respOff,
		responseCapacity: respCap,
		localReqSeq:      curReqSeq,
		localRespSeq:     curRespSeq,
		SpinTries:        shmDefaultSpin,
		ownerGeneration:  ownerGen,
		path:             path,
	}, nil
}

// ShmClose closes a client SHM context (no unlink).
func (c *ShmContext) ShmClose() {
	if c.data != nil {
		syscall.Munmap(c.data)
		c.data = nil
	}
	if c.fd >= 0 {
		syscall.Close(c.fd)
		c.fd = -1
	}
}

// ---------------------------------------------------------------------------
//  Data plane
// ---------------------------------------------------------------------------

// ShmSend publishes a message. The message must include the 32-byte
// outer header + payload, exactly as sent over UDS.
func (c *ShmContext) ShmSend(msg []byte) error {
	if c.data == nil || len(msg) == 0 {
		return fmt.Errorf("%w: null context or empty message", ErrShmBadParam)
	}

	var areaOff, areaCap uint32
	var seqOff, lenOff, sigOff int

	if c.role == ShmRoleClient {
		areaOff = c.requestOffset
		areaCap = c.requestCapacity
		seqOff = shmHeaderReqSeqOff
		lenOff = shmHeaderReqLenOff
		sigOff = shmHeaderReqSignalOff
	} else {
		areaOff = c.responseOffset
		areaCap = c.responseCapacity
		seqOff = shmHeaderRespSeqOff
		lenOff = shmHeaderRespLenOff
		sigOff = shmHeaderRespSignalOff
	}

	if uint32(len(msg)) > areaCap {
		return ErrShmMsgTooLarge
	}

	// 1. Write message data
	copy(c.data[areaOff:], msg)

	// 2. Store message length (release)
	if err := atomicStoreU32(c.data, lenOff, uint32(len(msg))); err != nil {
		return fmt.Errorf("%w: store msg_len: %v", ErrShmBadParam, err)
	}

	// 3. Increment sequence number (release)
	if err := atomicAddU64(c.data, seqOff, 1); err != nil {
		return fmt.Errorf("%w: add seq: %v", ErrShmBadParam, err)
	}

	// 4. Wake peer via futex
	if err := atomicAddU32(c.data, sigOff, 1); err != nil {
		return fmt.Errorf("%w: add signal: %v", ErrShmBadParam, err)
	}
	futexWakeCall(c.data, sigOff, 1)

	// Track locally
	if c.role == ShmRoleClient {
		c.localReqSeq++
	} else {
		c.localRespSeq++
	}

	return nil
}

// ShmReceive receives a message into the caller-provided buffer.
// On success, returns the number of bytes written to buf.
// Returns ErrShmMsgTooLarge if the message exceeds len(buf).
func (c *ShmContext) ShmReceive(buf []byte, timeoutMs uint32) (int, error) {
	if c.data == nil {
		return 0, fmt.Errorf("%w: null context", ErrShmBadParam)
	}
	if len(buf) == 0 {
		return 0, fmt.Errorf("%w: empty buffer", ErrShmBadParam)
	}

	var areaOff, areaCap uint32
	var seqOff, lenOff, sigOff int
	var expectedSeq uint64

	if c.role == ShmRoleServer {
		areaOff = c.requestOffset
		areaCap = c.requestCapacity
		seqOff = shmHeaderReqSeqOff
		lenOff = shmHeaderReqLenOff
		sigOff = shmHeaderReqSignalOff
		expectedSeq = c.localReqSeq + 1
	} else {
		areaOff = c.responseOffset
		areaCap = c.responseCapacity
		seqOff = shmHeaderRespSeqOff
		lenOff = shmHeaderRespLenOff
		sigOff = shmHeaderRespSignalOff
		expectedSeq = c.localRespSeq + 1
	}

	// Limit copy to the smaller of caller buffer and SHM area capacity
	maxCopy := min(int(areaCap), len(buf))

	// Phase 1: spin. Copy immediately on observing the advance.
	observed := false
	var mlen uint32
	for i := uint32(0); i < c.SpinTries; i++ {
		cur, err := atomicLoadU64(c.data, seqOff)
		if err != nil {
			return 0, fmt.Errorf("%w: load seq: %v", ErrShmBadParam, err)
		}
		if cur >= expectedSeq {
			mlen, err = atomicLoadU32(c.data, lenOff)
			if err != nil {
				return 0, fmt.Errorf("%w: load msg_len: %v", ErrShmBadParam, err)
			}
			if mlen > 0 && int(mlen) <= maxCopy {
				copy(buf[:mlen], c.data[areaOff:areaOff+mlen])
			}
			observed = true
			break
		}
		spinPause()
	}

	// Phase 2: futex wait with deadline-based retry loop.
	//
	// Handles spurious wakeups (EAGAIN when signal word changed
	// between read and syscall, or EINTR from signal delivery).
	// Computes a wall-clock deadline so total wait never exceeds
	// timeoutMs regardless of retries.
	if !observed {
		var deadlineNs uint64
		if timeoutMs > 0 {
			var nowTs syscall.Timespec
			syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1 /* CLOCK_MONOTONIC */, uintptr(unsafe.Pointer(&nowTs)), 0)
			deadlineNs = uint64(nowTs.Sec)*1_000_000_000 + uint64(nowTs.Nsec) +
				uint64(timeoutMs)*1_000_000
		}

		for {
			sigVal, serr := atomicLoadU32(c.data, sigOff)
			if serr != nil {
				return 0, fmt.Errorf("%w: load signal: %v", ErrShmBadParam, serr)
			}

			cur, serr := atomicLoadU64(c.data, seqOff)
			if serr != nil {
				return 0, fmt.Errorf("%w: load seq: %v", ErrShmBadParam, serr)
			}
			if cur >= expectedSeq {
				break // response arrived
			}

			// Compute remaining timeout for this futex_wait call
			var ts *syscall.Timespec
			if deadlineNs > 0 {
				var nowTs syscall.Timespec
				syscall.Syscall(syscall.SYS_CLOCK_GETTIME, 1 /* CLOCK_MONOTONIC */, uintptr(unsafe.Pointer(&nowTs)), 0)
				nowVal := uint64(nowTs.Sec)*1_000_000_000 + uint64(nowTs.Nsec)
				if nowVal >= deadlineNs {
					return 0, ErrShmTimeout
				}
				remain := deadlineNs - nowVal
				ts = &syscall.Timespec{
					Sec:  int64(remain / 1_000_000_000),
					Nsec: int64(remain % 1_000_000_000),
				}
			}

			ret := futexWaitCall(c.data, sigOff, sigVal, ts)
			if ret < 0 {
				errno := syscall.Errno(-ret)
				if errno == syscall.ETIMEDOUT {
					return 0, ErrShmTimeout
				}
			}

			// EAGAIN (value changed) or EINTR (signal): re-check seq
		}

		// Copy immediately after observing the sequence advance
		var lerr error
		mlen, lerr = atomicLoadU32(c.data, lenOff)
		if lerr != nil {
			return 0, fmt.Errorf("%w: load msg_len: %v", ErrShmBadParam, lerr)
		}
		if mlen > 0 && int(mlen) <= maxCopy {
			copy(buf[:mlen], c.data[areaOff:areaOff+mlen])
		}
	}

	// Advance local tracking (message is consumed from SHM perspective)
	if c.role == ShmRoleServer {
		c.localReqSeq = expectedSeq
	} else {
		c.localRespSeq = expectedSeq
	}

	// Message larger than safe copy limit
	if int(mlen) > maxCopy {
		return int(mlen), ErrShmMsgTooLarge
	}

	return int(mlen), nil
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

func shmAlign64(v uint32) uint32 {
	return (v + (shmRegionAlignment - 1)) & ^(shmRegionAlignment - 1)
}

// validateShmServiceName checks that name contains only [a-zA-Z0-9._-],
// is non-empty, and is not "." or "..".
func validateShmServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty service name", ErrShmBadParam)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: service name cannot be '.' or '..'", ErrShmBadParam)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' {
			continue
		}
		return fmt.Errorf("%w: service name contains invalid character: %q", ErrShmBadParam, c)
	}
	return nil
}

func buildShmPath(runDir, serviceName string, sessionID uint64) (string, error) {
	if err := validateShmServiceName(serviceName); err != nil {
		return "", err
	}
	path := filepath.Join(runDir, fmt.Sprintf("%s-%016x.ipcshm", serviceName, sessionID))
	if len(path) >= shmMaxPath {
		return "", ErrShmPathTooLong
	}
	return path, nil
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Atomic operations on the mmap'd region with bounds checking.

var errShmOutOfBounds = errors.New("SHM atomic: offset out of bounds")

func atomicLoadU64(data []byte, off int) (uint64, error) {
	if off < 0 || off+8 > len(data) {
		return 0, errShmOutOfBounds
	}
	ptr := (*uint64)(unsafe.Pointer(&data[off]))
	return atomic.LoadUint64(ptr), nil
}

func atomicLoadU32(data []byte, off int) (uint32, error) {
	if off < 0 || off+4 > len(data) {
		return 0, errShmOutOfBounds
	}
	ptr := (*uint32)(unsafe.Pointer(&data[off]))
	return atomic.LoadUint32(ptr), nil
}

func atomicStoreU32(data []byte, off int, val uint32) error {
	if off < 0 || off+4 > len(data) {
		return errShmOutOfBounds
	}
	ptr := (*uint32)(unsafe.Pointer(&data[off]))
	atomic.StoreUint32(ptr, val)
	return nil
}

func atomicAddU64(data []byte, off int, val uint64) error {
	if off < 0 || off+8 > len(data) {
		return errShmOutOfBounds
	}
	ptr := (*uint64)(unsafe.Pointer(&data[off]))
	atomic.AddUint64(ptr, val)
	return nil
}

func atomicAddU32(data []byte, off int, val uint32) error {
	if off < 0 || off+4 > len(data) {
		return errShmOutOfBounds
	}
	ptr := (*uint32)(unsafe.Pointer(&data[off]))
	atomic.AddUint32(ptr, val)
	return nil
}

func futexWakeCall(data []byte, off int, count int) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off])
	r1, _, _ := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(addr),
		uintptr(futexWake),
		uintptr(count),
		0, 0, 0,
	)
	return int(r1)
}

func futexWaitCall(data []byte, off int, expected uint32, ts *syscall.Timespec) int {
	if off < 0 || off+4 > len(data) {
		return -1
	}
	addr := unsafe.Pointer(&data[off])
	var tsPtr uintptr
	if ts != nil {
		tsPtr = uintptr(unsafe.Pointer(ts))
	}
	r1, _, errno := syscall.Syscall6(
		syscall.SYS_FUTEX,
		uintptr(addr),
		uintptr(futexWait),
		uintptr(expected),
		tsPtr,
		0, 0,
	)
	if errno != 0 {
		return -int(errno)
	}
	return int(r1)
}

// ---------------------------------------------------------------------------
//  Stale cleanup
// ---------------------------------------------------------------------------

// ShmCleanupStale scans runDir for SHM files matching {serviceName}-*.ipcshm,
// checks if the owner PID is alive for each, and unlinks stale ones.
func ShmCleanupStale(runDir, serviceName string) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return
	}
	prefix := serviceName + "-"
	suffix := ".ipcshm"
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() {
			continue
		}
		if len(name) < len(prefix)+len(suffix) {
			continue
		}
		if name[:len(prefix)] != prefix || name[len(name)-len(suffix):] != suffix {
			continue
		}
		path := filepath.Join(runDir, name)
		result := checkShmStale(path)
		_ = result // checkShmStale already unlinks stale/invalid files
	}
}

// ---------------------------------------------------------------------------
//  Stale region recovery
// ---------------------------------------------------------------------------

type shmStaleResult int

const (
	shmStaleNotExist shmStaleResult = iota
	shmStaleRecovered
	shmStaleLive
	shmStaleInvalid
)

func checkShmStale(path string) shmStaleResult {
	info, err := os.Stat(path)
	if err != nil {
		return shmStaleNotExist
	}

	if info.Size() < int64(shmHeaderLen) {
		os.Remove(path)
		return shmStaleInvalid
	}

	f, err := os.Open(path)
	if err != nil {
		os.Remove(path)
		return shmStaleInvalid
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, int(shmHeaderLen),
		syscall.PROT_READ, syscall.MAP_SHARED)
	f.Close()
	if err != nil {
		os.Remove(path)
		return shmStaleInvalid
	}

	magic := binary.NativeEndian.Uint32(data[shmHeaderMagicOff : shmHeaderMagicOff+4])
	if magic != shmRegionMagic {
		syscall.Munmap(data)
		os.Remove(path)
		return shmStaleInvalid
	}

	ownerPid := int(int32(binary.NativeEndian.Uint32(data[shmHeaderOwnerPidOff : shmHeaderOwnerPidOff+4])))
	ownerGen := binary.NativeEndian.Uint32(data[shmHeaderOwnerGenOff : shmHeaderOwnerGenOff+4])
	syscall.Munmap(data)

	if pidAlive(ownerPid) && ownerGen != 0 {
		return shmStaleLive
	}

	// Dead owner or zero generation (PID reuse / legacy) — stale
	os.Remove(path)
	return shmStaleRecovered
}
