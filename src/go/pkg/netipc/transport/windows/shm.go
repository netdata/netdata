//go:build windows

// Windows SHM transport — shared memory data plane with spin + kernel event
// synchronization. Wire-compatible with the C and Rust implementations.
//
// Pure Go — no cgo. Works with CGO_ENABLED=0.

package windows

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const (
	winShmMagic         uint32 = 0x4e535748 // "NSWH"
	winShmVersion       uint32 = 3
	winShmHeaderLen     uint32 = 128
	winShmCachelineSize uint32 = 64
	winShmDefaultSpin   uint32 = 1024

	WinShmProfileHybrid   uint32 = 0x02
	WinShmProfileBusywait uint32 = 0x04

	// Header field offsets
	wshOFFMagic             = 0
	wshOFFVersion           = 4
	wshOFFHeaderLen         = 8
	wshOFFProfile           = 12
	wshOFFReqOffset         = 16
	wshOFFReqCapacity       = 20
	wshOFFRespOffset        = 24
	wshOFFRespCapacity      = 28
	wshOFFSpinTries         = 32
	wshOFFReqLen            = 36
	wshOFFRespLen           = 40
	wshOFFReqClientClosed   = 44
	wshOFFReqServerWaiting  = 48
	wshOFFRespServerClosed  = 52
	wshOFFRespClientWaiting = 56
	wshOFFReqSeq            = 64
	wshOFFRespSeq           = 72
)

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrWinShmBadParam      = errors.New("invalid Windows SHM argument")
	ErrWinShmCreateMapping = errors.New("CreateFileMappingW failed")
	ErrWinShmOpenMapping   = errors.New("OpenFileMappingW failed")
	ErrWinShmMapView       = errors.New("MapViewOfFile failed")
	ErrWinShmCreateEvent   = errors.New("CreateEventW failed")
	ErrWinShmOpenEvent     = errors.New("OpenEventW failed")
	ErrWinShmAddrInUse     = errors.New("Windows SHM object name already in use by live server")
	ErrWinShmBadMagic      = errors.New("Windows SHM header magic mismatch")
	ErrWinShmBadVersion    = errors.New("Windows SHM header version mismatch")
	ErrWinShmBadHeader     = errors.New("Windows SHM header_len mismatch")
	ErrWinShmBadProfile    = errors.New("Windows SHM profile mismatch")
	ErrWinShmMsgTooLarge   = errors.New("message exceeds Windows SHM area capacity")
	ErrWinShmTimeout       = errors.New("Windows SHM wait timed out")
	ErrWinShmDisconnected  = errors.New("Windows SHM peer closed")
)

// ---------------------------------------------------------------------------
//  Win32 syscall imports
// ---------------------------------------------------------------------------

var (
	procCreateFileMappingW = modkernel32.NewProc("CreateFileMappingW")
	procOpenFileMappingW   = modkernel32.NewProc("OpenFileMappingW")
	procMapViewOfFile      = modkernel32.NewProc("MapViewOfFile")
	procUnmapViewOfFile    = modkernel32.NewProc("UnmapViewOfFile")
	procCreateEventW       = modkernel32.NewProc("CreateEventW")
	procOpenEventW         = modkernel32.NewProc("OpenEventW")
	procSetEvent           = modkernel32.NewProc("SetEvent")
	procWaitForSingleObj   = modkernel32.NewProc("WaitForSingleObject")
	procGetTickCount64     = modkernel32.NewProc("GetTickCount64")
)

type winShmProcCall func(a ...uintptr) (uintptr, uintptr, error)

func callCreateFileMappingW(a ...uintptr) (uintptr, uintptr, error) {
	return procCreateFileMappingW.Call(a...)
}
func callOpenFileMappingW(a ...uintptr) (uintptr, uintptr, error) {
	return procOpenFileMappingW.Call(a...)
}
func callMapViewOfFile(a ...uintptr) (uintptr, uintptr, error) { return procMapViewOfFile.Call(a...) }
func callCreateEventW(a ...uintptr) (uintptr, uintptr, error)  { return procCreateEventW.Call(a...) }
func callOpenEventW(a ...uintptr) (uintptr, uintptr, error)    { return procOpenEventW.Call(a...) }

var (
	winShmCreateFileMappingW winShmProcCall = callCreateFileMappingW
	winShmOpenFileMappingW   winShmProcCall = callOpenFileMappingW
	winShmMapViewOfFile      winShmProcCall = callMapViewOfFile
	winShmCreateEventW       winShmProcCall = callCreateEventW
	winShmOpenEventW         winShmProcCall = callOpenEventW
)

const (
	_PAGE_READWRITE       = 0x04
	_FILE_MAP_ALL_ACCESS  = 0x000F001F
	_EVENT_MODIFY_STATE   = 0x0002
	_SYNCHRONIZE          = 0x00100000
	_INFINITE             = 0xFFFFFFFF
	_WAIT_TIMEOUT         = 0x00000102
	_ERROR_ALREADY_EXISTS = 183
)

func isWindowsErrno(err error, want syscall.Errno) bool {
	errno, ok := err.(syscall.Errno)
	return ok && errno == want
}

// ---------------------------------------------------------------------------
//  Role
// ---------------------------------------------------------------------------

// WinShmRole distinguishes server vs client.
type WinShmRole int

const (
	WinShmRoleServer WinShmRole = 1
	WinShmRoleClient WinShmRole = 2
)

// ---------------------------------------------------------------------------
//  Context
// ---------------------------------------------------------------------------

// WinShmContext is a handle to a Windows SHM region.
type WinShmContext struct {
	role    WinShmRole
	mapping syscall.Handle
	base    uintptr
	size    uintptr

	reqEvent  syscall.Handle
	respEvent syscall.Handle

	profile          uint32
	requestOffset    uint32
	requestCapacity  uint32
	responseOffset   uint32
	responseCapacity uint32
	SpinTries        uint32

	localReqSeq  int64
	localRespSeq int64
}

// Role returns the context role.
func (c *WinShmContext) GetRole() WinShmRole { return c.role }

// ---------------------------------------------------------------------------
//  Server API
// ---------------------------------------------------------------------------

// WinShmServerCreate creates a per-session Windows SHM region.
func WinShmServerCreate(runDir, serviceName string, authToken, sessionID uint64,
	profile, reqCapacity, respCapacity uint32) (*WinShmContext, error) {

	if err := validateServiceName(serviceName); err != nil {
		return nil, err
	}
	if err := validateWinShmProfile(profile); err != nil {
		return nil, err
	}

	hash := computeShmHash(runDir, serviceName, authToken)
	mappingName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "mapping")
	if err != nil {
		return nil, err
	}

	reqCap := winShmAlignCacheline(reqCapacity)
	respCap := winShmAlignCacheline(respCapacity)
	reqOff := winShmAlignCacheline(winShmHeaderLen)
	respOff := winShmAlignCacheline(reqOff + reqCap)
	regionSize := uintptr(respOff + respCap)

	// Create page-file backed mapping
	r, _, callErr := winShmCreateFileMappingW(
		uintptr(syscall.InvalidHandle), // page file
		0,                              // NULL security
		uintptr(_PAGE_READWRITE),
		uintptr(regionSize>>32),
		uintptr(regionSize&0xFFFFFFFF),
		uintptr(unsafe.Pointer(&mappingName[0])),
	)
	mapping := syscall.Handle(r)
	if mapping == 0 {
		return nil, fmt.Errorf("%w: %v", ErrWinShmCreateMapping, callErr)
	}
	if isWindowsErrno(callErr, syscall.Errno(_ERROR_ALREADY_EXISTS)) {
		syscall.CloseHandle(mapping)
		return nil, ErrWinShmAddrInUse
	}

	// Map view
	base, _, callErr := winShmMapViewOfFile(
		uintptr(mapping),
		uintptr(_FILE_MAP_ALL_ACCESS),
		0, 0,
		regionSize,
	)
	if base == 0 {
		syscall.CloseHandle(mapping)
		return nil, fmt.Errorf("%w: %v", ErrWinShmMapView, callErr)
	}

	// Zero region
	data := unsafe.Slice((*byte)(unsafe.Pointer(base)), regionSize)
	for i := range data {
		data[i] = 0
	}

	// Write header
	binary.NativeEndian.PutUint32(data[wshOFFMagic:], winShmMagic)
	binary.NativeEndian.PutUint32(data[wshOFFVersion:], winShmVersion)
	binary.NativeEndian.PutUint32(data[wshOFFHeaderLen:], winShmHeaderLen)
	binary.NativeEndian.PutUint32(data[wshOFFProfile:], profile)
	binary.NativeEndian.PutUint32(data[wshOFFReqOffset:], reqOff)
	binary.NativeEndian.PutUint32(data[wshOFFReqCapacity:], reqCap)
	binary.NativeEndian.PutUint32(data[wshOFFRespOffset:], respOff)
	binary.NativeEndian.PutUint32(data[wshOFFRespCapacity:], respCap)
	binary.NativeEndian.PutUint32(data[wshOFFSpinTries:], winShmDefaultSpin)

	// Release fence
	atomic.StoreInt32((*int32)(unsafe.Pointer(&data[wshOFFReqLen])), 0)

	// Create events for HYBRID
	var reqEvent, respEvent syscall.Handle
	reqEvent = syscall.InvalidHandle
	respEvent = syscall.InvalidHandle

	if profile == WinShmProfileHybrid {
		reqEventName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "req_event")
		if err != nil {
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, err
		}

		r, _, callErr := winShmCreateEventW(0, 0, 0,
			uintptr(unsafe.Pointer(&reqEventName[0])))
		if r == 0 {
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, fmt.Errorf("%w: req_event: %v", ErrWinShmCreateEvent, callErr)
		}
		reqEvent = syscall.Handle(r)
		if isWindowsErrno(callErr, syscall.Errno(_ERROR_ALREADY_EXISTS)) {
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, ErrWinShmAddrInUse
		}

		respEventName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "resp_event")
		if err != nil {
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, err
		}

		r, _, callErr = winShmCreateEventW(0, 0, 0,
			uintptr(unsafe.Pointer(&respEventName[0])))
		if r == 0 {
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, fmt.Errorf("%w: resp_event: %v", ErrWinShmCreateEvent, callErr)
		}
		respEvent = syscall.Handle(r)
		if isWindowsErrno(callErr, syscall.Errno(_ERROR_ALREADY_EXISTS)) {
			syscall.CloseHandle(respEvent)
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, ErrWinShmAddrInUse
		}
	}

	return &WinShmContext{
		role:             WinShmRoleServer,
		mapping:          mapping,
		base:             base,
		size:             regionSize,
		reqEvent:         reqEvent,
		respEvent:        respEvent,
		profile:          profile,
		requestOffset:    reqOff,
		requestCapacity:  reqCap,
		responseOffset:   respOff,
		responseCapacity: respCap,
		SpinTries:        winShmDefaultSpin,
		localReqSeq:      0,
		localRespSeq:     0,
	}, nil
}

// WinShmDestroy destroys a server SHM region.
func (c *WinShmContext) WinShmDestroy() {
	if c.base != 0 {
		data := unsafe.Slice((*byte)(unsafe.Pointer(c.base)), c.size)
		atomic.StoreInt32((*int32)(unsafe.Pointer(&data[wshOFFRespServerClosed])), 1)
	}

	if c.profile == WinShmProfileHybrid && c.respEvent != syscall.InvalidHandle {
		procSetEvent.Call(uintptr(c.respEvent))
	}

	c.cleanupHandles()
}

// ---------------------------------------------------------------------------
//  Client API
// ---------------------------------------------------------------------------

// WinShmClientAttach attaches to an existing per-session Windows SHM region.
func WinShmClientAttach(runDir, serviceName string, authToken, sessionID uint64,
	profile uint32) (*WinShmContext, error) {

	if err := validateServiceName(serviceName); err != nil {
		return nil, err
	}
	if err := validateWinShmProfile(profile); err != nil {
		return nil, err
	}

	hash := computeShmHash(runDir, serviceName, authToken)
	mappingName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "mapping")
	if err != nil {
		return nil, err
	}

	r, _, callErr := winShmOpenFileMappingW(
		uintptr(_FILE_MAP_ALL_ACCESS),
		0,
		uintptr(unsafe.Pointer(&mappingName[0])),
	)
	mapping := syscall.Handle(r)
	if mapping == 0 {
		return nil, fmt.Errorf("%w: %v", ErrWinShmOpenMapping, callErr)
	}

	base, _, callErr := winShmMapViewOfFile(
		uintptr(mapping),
		uintptr(_FILE_MAP_ALL_ACCESS),
		0, 0, 0,
	)
	if base == 0 {
		syscall.CloseHandle(mapping)
		return nil, fmt.Errorf("%w: %v", ErrWinShmMapView, callErr)
	}

	// We need at least header_len to validate
	data := unsafe.Slice((*byte)(unsafe.Pointer(base)), winShmHeaderLen)

	// Acquire fence
	atomic.LoadInt32((*int32)(unsafe.Pointer(&data[wshOFFReqLen])))

	// Validate header
	magic := binary.NativeEndian.Uint32(data[wshOFFMagic:])
	if magic != winShmMagic {
		procUnmapViewOfFile.Call(base)
		syscall.CloseHandle(mapping)
		return nil, ErrWinShmBadMagic
	}
	version := binary.NativeEndian.Uint32(data[wshOFFVersion:])
	if version != winShmVersion {
		procUnmapViewOfFile.Call(base)
		syscall.CloseHandle(mapping)
		return nil, ErrWinShmBadVersion
	}
	hdrLen := binary.NativeEndian.Uint32(data[wshOFFHeaderLen:])
	if hdrLen != winShmHeaderLen {
		procUnmapViewOfFile.Call(base)
		syscall.CloseHandle(mapping)
		return nil, ErrWinShmBadHeader
	}
	hdrProfile := binary.NativeEndian.Uint32(data[wshOFFProfile:])
	if hdrProfile != profile {
		procUnmapViewOfFile.Call(base)
		syscall.CloseHandle(mapping)
		return nil, ErrWinShmBadProfile
	}

	reqOff := binary.NativeEndian.Uint32(data[wshOFFReqOffset:])
	reqCap := binary.NativeEndian.Uint32(data[wshOFFReqCapacity:])
	respOff := binary.NativeEndian.Uint32(data[wshOFFRespOffset:])
	respCap := binary.NativeEndian.Uint32(data[wshOFFRespCapacity:])
	spin := binary.NativeEndian.Uint32(data[wshOFFSpinTries:])
	regionSize := uintptr(respOff + respCap)

	// Now reslice to full region
	fullData := unsafe.Slice((*byte)(unsafe.Pointer(base)), regionSize)

	// Read current sequence numbers via atomic
	curReqSeq := atomic.LoadInt64((*int64)(unsafe.Pointer(&fullData[wshOFFReqSeq])))
	curRespSeq := atomic.LoadInt64((*int64)(unsafe.Pointer(&fullData[wshOFFRespSeq])))

	// Open events for HYBRID
	var reqEvent, respEvent syscall.Handle
	reqEvent = syscall.InvalidHandle
	respEvent = syscall.InvalidHandle

	if profile == WinShmProfileHybrid {
		reqEventName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "req_event")
		if err != nil {
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, err
		}

		r, _, callErr := winShmOpenEventW(
			uintptr(_EVENT_MODIFY_STATE|_SYNCHRONIZE),
			0,
			uintptr(unsafe.Pointer(&reqEventName[0])),
		)
		if r == 0 {
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, fmt.Errorf("%w: req_event: %v", ErrWinShmOpenEvent, callErr)
		}
		reqEvent = syscall.Handle(r)

		respEventName, err := buildWinShmObjectName(hash, serviceName, profile, sessionID, "resp_event")
		if err != nil {
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, err
		}

		r, _, callErr = winShmOpenEventW(
			uintptr(_EVENT_MODIFY_STATE|_SYNCHRONIZE),
			0,
			uintptr(unsafe.Pointer(&respEventName[0])),
		)
		if r == 0 {
			syscall.CloseHandle(reqEvent)
			procUnmapViewOfFile.Call(base)
			syscall.CloseHandle(mapping)
			return nil, fmt.Errorf("%w: resp_event: %v", ErrWinShmOpenEvent, callErr)
		}
		respEvent = syscall.Handle(r)
	}

	return &WinShmContext{
		role:             WinShmRoleClient,
		mapping:          mapping,
		base:             base,
		size:             regionSize,
		reqEvent:         reqEvent,
		respEvent:        respEvent,
		profile:          profile,
		requestOffset:    reqOff,
		requestCapacity:  reqCap,
		responseOffset:   respOff,
		responseCapacity: respCap,
		SpinTries:        spin,
		localReqSeq:      curReqSeq,
		localRespSeq:     curRespSeq,
	}, nil
}

// WinShmClose closes a client SHM context.
func (c *WinShmContext) WinShmClose() {
	if c.base != 0 {
		data := unsafe.Slice((*byte)(unsafe.Pointer(c.base)), c.size)
		atomic.StoreInt32((*int32)(unsafe.Pointer(&data[wshOFFReqClientClosed])), 1)
	}

	if c.profile == WinShmProfileHybrid && c.reqEvent != syscall.InvalidHandle {
		procSetEvent.Call(uintptr(c.reqEvent))
	}

	c.cleanupHandles()
}

func (c *WinShmContext) cleanupHandles() {
	if c.base != 0 {
		procUnmapViewOfFile.Call(c.base)
		c.base = 0
	}
	if c.mapping != syscall.InvalidHandle && c.mapping != 0 {
		syscall.CloseHandle(c.mapping)
		c.mapping = syscall.InvalidHandle
	}
	if c.reqEvent != syscall.InvalidHandle {
		syscall.CloseHandle(c.reqEvent)
		c.reqEvent = syscall.InvalidHandle
	}
	if c.respEvent != syscall.InvalidHandle {
		syscall.CloseHandle(c.respEvent)
		c.respEvent = syscall.InvalidHandle
	}
	c.size = 0
}

// ---------------------------------------------------------------------------
//  Data plane
// ---------------------------------------------------------------------------

// WinShmSend publishes a message. The message must include the 32-byte
// outer header + payload, exactly as sent over Named Pipe.
func (c *WinShmContext) WinShmSend(msg []byte) error {
	if c.base == 0 || len(msg) == 0 {
		return fmt.Errorf("%w: null context or empty message", ErrWinShmBadParam)
	}

	var areaOff, areaCap uint32
	var lenOff, seqOff, peerWaitingOff int
	var peerEvent syscall.Handle

	if c.role == WinShmRoleClient {
		areaOff = c.requestOffset
		areaCap = c.requestCapacity
		lenOff = wshOFFReqLen
		seqOff = wshOFFReqSeq
		peerWaitingOff = wshOFFReqServerWaiting
		peerEvent = c.reqEvent
	} else {
		areaOff = c.responseOffset
		areaCap = c.responseCapacity
		lenOff = wshOFFRespLen
		seqOff = wshOFFRespSeq
		peerWaitingOff = wshOFFRespClientWaiting
		peerEvent = c.respEvent
	}

	if uint32(len(msg)) > areaCap {
		return ErrWinShmMsgTooLarge
	}

	data := unsafe.Slice((*byte)(unsafe.Pointer(c.base)), c.size)

	// 1. Write message data
	copy(data[areaOff:], msg)

	// 2. Store message length (atomic)
	atomic.StoreInt32((*int32)(unsafe.Pointer(&data[lenOff])), int32(len(msg)))

	// 3. Increment sequence number (atomic)
	atomic.AddInt64((*int64)(unsafe.Pointer(&data[seqOff])), 1)

	// 4. If HYBRID and peer waiting, signal event
	if c.profile == WinShmProfileHybrid {
		if atomic.LoadInt32((*int32)(unsafe.Pointer(&data[peerWaitingOff]))) != 0 {
			procSetEvent.Call(uintptr(peerEvent))
		}
	}

	if c.role == WinShmRoleClient {
		c.localReqSeq++
	} else {
		c.localRespSeq++
	}

	return nil
}

// WinShmReceive receives a message into the caller-provided buffer.
func (c *WinShmContext) WinShmReceive(buf []byte, timeoutMs uint32) (int, error) {
	if c.base == 0 {
		return 0, fmt.Errorf("%w: null context", ErrWinShmBadParam)
	}
	if len(buf) == 0 {
		return 0, fmt.Errorf("%w: empty buffer", ErrWinShmBadParam)
	}

	var areaOff, areaCap uint32
	var lenOff, seqOff, selfWaitingOff, peerClosedOff int
	var waitEvent syscall.Handle
	var expectedSeq int64

	if c.role == WinShmRoleServer {
		areaOff = c.requestOffset
		areaCap = c.requestCapacity
		lenOff = wshOFFReqLen
		seqOff = wshOFFReqSeq
		selfWaitingOff = wshOFFReqServerWaiting
		peerClosedOff = wshOFFReqClientClosed
		waitEvent = c.reqEvent
		expectedSeq = c.localReqSeq + 1
	} else {
		areaOff = c.responseOffset
		areaCap = c.responseCapacity
		lenOff = wshOFFRespLen
		seqOff = wshOFFRespSeq
		selfWaitingOff = wshOFFRespClientWaiting
		peerClosedOff = wshOFFRespServerClosed
		waitEvent = c.respEvent
		expectedSeq = c.localRespSeq + 1
	}

	// The copy ceiling is the smaller of the caller buffer and the
	// SHM area capacity. Prevents out-of-bounds reads on forged lengths.
	maxCopy := len(buf)
	if int(areaCap) < maxCopy {
		maxCopy = int(areaCap)
	}

	data := unsafe.Slice((*byte)(unsafe.Pointer(c.base)), c.size)

	// Phase 1: spin
	observed := false
	var mlen int32
	for i := uint32(0); i < c.SpinTries; i++ {
		cur := atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
		if cur >= expectedSeq {
			mlen = atomic.LoadInt32((*int32)(unsafe.Pointer(&data[lenOff])))
			if mlen > 0 && int(mlen) <= maxCopy {
				copy(buf[:mlen], data[areaOff:areaOff+uint32(mlen)])
			}
			observed = true
			break
		}
		spinPause()
	}

	// Phase 2: kernel wait or busy-wait
	if !observed {
		if c.profile == WinShmProfileHybrid {
			deadlineMs := uint32(_INFINITE)
			if timeoutMs > 0 {
				deadlineMs = timeoutMs
			}
			start, _, _ := procGetTickCount64.Call()

			for {
				atomic.StoreInt32((*int32)(unsafe.Pointer(&data[selfWaitingOff])), 1)
				atomic.LoadInt32((*int32)(unsafe.Pointer(&data[selfWaitingOff])))

				cur := atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
				if cur >= expectedSeq {
					atomic.StoreInt32((*int32)(unsafe.Pointer(&data[selfWaitingOff])), 0)
					break
				}

				waitMs := uintptr(_INFINITE)
				if deadlineMs != _INFINITE {
					now, _, _ := procGetTickCount64.Call()
					elapsed := uint32(now - start)
					if elapsed >= deadlineMs {
						atomic.StoreInt32((*int32)(unsafe.Pointer(&data[selfWaitingOff])), 0)
						return 0, ErrWinShmTimeout
					}
					waitMs = uintptr(deadlineMs - elapsed)
				}

				ret, _, _ := procWaitForSingleObj.Call(uintptr(waitEvent), waitMs)
				atomic.StoreInt32((*int32)(unsafe.Pointer(&data[selfWaitingOff])), 0)

				cur = atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
				if cur >= expectedSeq {
					break
				}

				if atomic.LoadInt32((*int32)(unsafe.Pointer(&data[peerClosedOff]))) != 0 {
					cur = atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
					if cur >= expectedSeq {
						break
					}
					c.advanceSeq(expectedSeq)
					return 0, ErrWinShmDisconnected
				}

				if ret == _WAIT_TIMEOUT {
					return 0, ErrWinShmTimeout
				}
			}

			mlen = atomic.LoadInt32((*int32)(unsafe.Pointer(&data[lenOff])))
			if mlen > 0 && int(mlen) <= maxCopy {
				copy(buf[:mlen], data[areaOff:areaOff+uint32(mlen)])
			}
		} else {
			// BUSYWAIT
			start, _, _ := procGetTickCount64.Call()
			for {
				cur := atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
				if cur >= expectedSeq {
					mlen = atomic.LoadInt32((*int32)(unsafe.Pointer(&data[lenOff])))
					if mlen > 0 && int(mlen) <= maxCopy {
						copy(buf[:mlen], data[areaOff:areaOff+uint32(mlen)])
					}
					break
				}

				if timeoutMs > 0 {
					now, _, _ := procGetTickCount64.Call()
					elapsed := uint64(now - start)
					if elapsed >= uint64(timeoutMs) {
						return 0, ErrWinShmTimeout
					}
				}

				if atomic.LoadInt32((*int32)(unsafe.Pointer(&data[peerClosedOff]))) != 0 {
					cur := atomic.LoadInt64((*int64)(unsafe.Pointer(&data[seqOff])))
					if cur >= expectedSeq {
						mlen = atomic.LoadInt32((*int32)(unsafe.Pointer(&data[lenOff])))
						if mlen > 0 && int(mlen) <= maxCopy {
							copy(buf[:mlen], data[areaOff:areaOff+uint32(mlen)])
						}
						break
					}
					c.advanceSeq(expectedSeq)
					return 0, ErrWinShmDisconnected
				}

				spinPause()
			}
		}
	}

	c.advanceSeq(expectedSeq)

	if int(mlen) > maxCopy {
		return int(mlen), ErrWinShmMsgTooLarge
	}

	return int(mlen), nil
}

func (c *WinShmContext) advanceSeq(expectedSeq int64) {
	if c.role == WinShmRoleServer {
		c.localReqSeq = expectedSeq
	} else {
		c.localRespSeq = expectedSeq
	}
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

func winShmAlignCacheline(v uint32) uint32 {
	return (v + (winShmCachelineSize - 1)) & ^(winShmCachelineSize - 1)
}

func validateWinShmProfile(profile uint32) error {
	if profile != WinShmProfileHybrid && profile != WinShmProfileBusywait {
		return fmt.Errorf("%w: invalid profile %d", ErrWinShmBadParam, profile)
	}
	return nil
}

func computeShmHash(runDir, serviceName string, authToken uint64) uint64 {
	input := fmt.Sprintf("%s\n%s\n%d", runDir, serviceName, authToken)
	return FNV1a64([]byte(input))
}

func buildWinShmObjectName(hash uint64, serviceName string,
	profile uint32, sessionID uint64, suffix string) ([]uint16, error) {

	narrow := fmt.Sprintf(`Local\netipc-%016x-%s-p%d-s%016x-%s`,
		hash, serviceName, profile, sessionID, suffix)
	if len(narrow) >= 256 {
		return nil, fmt.Errorf("%w: object name too long", ErrWinShmBadParam)
	}

	// Convert to NUL-terminated UTF-16
	runes := []rune(narrow)
	runes = append(runes, 0)
	utf16 := make([]uint16, len(runes))
	for i, r := range runes {
		utf16[i] = uint16(r)
	}
	return utf16, nil
}
