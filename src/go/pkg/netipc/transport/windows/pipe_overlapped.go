//go:build windows

package windows

import (
	"encoding/binary"
	"fmt"
	"math"
	"syscall"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Client pipe handles are overlapped (FILE_FLAG_OVERLAPPED), so a
// pipelining send can read responses while its write waits for buffer
// space. Every read and write on such a handle passes an OVERLAPPED and is
// waited to completion before the call returns: no I/O stays posted between
// calls. Server handles stay synchronous.

// progressYield is how long a stalled pipelined send keeps yielding before
// it sleeps.
const progressYield = time.Millisecond

const (
	_FILE_FLAG_OVERLAPPED = 0x40000000
	_ERROR_IO_PENDING     = 997
	_ERROR_IO_INCOMPLETE  = 996
	_WAIT_OBJECT_0        = 0
)

var procGetOverlappedResult = modkernel32.NewProc("GetOverlappedResult")

// getOverlappedResult runs per message; SyscallN keeps its arguments on the
// stack.
func getOverlappedResult(handle syscall.Handle, ov *syscall.Overlapped, done *uint32, wait bool) error {
	var w uintptr
	if wait {
		w = 1
	}
	r, _, err := syscall.SyscallN(procGetOverlappedResult.Addr(),
		uintptr(handle), uintptr(unsafe.Pointer(ov)), uintptr(unsafe.Pointer(done)), w)
	if r == 0 {
		return err
	}
	return nil
}

// createManualEvent creates an unnamed, manual-reset, non-signalled event.
func createManualEvent() (syscall.Handle, error) {
	r, _, err := procCreateEventW.Call(0, 1, 0, 0)
	if r == 0 {
		return syscall.InvalidHandle, err
	}
	return syscall.Handle(r), nil
}

// pipeIO is the overlapped state of a client handle. It lives on the heap:
// the kernel writes to an OVERLAPPED and its buffer while the I/O is posted,
// and goroutine stacks can move in that time. Holding the buffers here makes
// every buffer passed to the kernel escape to the heap, where it stays put.
type pipeIO struct {
	read     syscall.Overlapped
	write    syscall.Overlapped
	readBuf  []byte
	writeBuf []byte
}

func newPipeIO() (*pipeIO, error) {
	io := &pipeIO{}
	readEvent, err := createManualEvent()
	if err != nil {
		return nil, wrapErr(ErrConnect, "CreateEvent: "+err.Error())
	}
	writeEvent, err := createManualEvent()
	if err != nil {
		syscall.CloseHandle(readEvent)
		return nil, wrapErr(ErrConnect, "CreateEvent: "+err.Error())
	}
	io.read.HEvent = readEvent
	io.write.HEvent = writeEvent
	return io, nil
}

func (io *pipeIO) close() {
	if io == nil {
		return
	}
	syscall.CloseHandle(io.read.HEvent)
	syscall.CloseHandle(io.write.HEvent)
}

func sendError(err error) error {
	if isDisconnectError(err) {
		return ErrDisconnected
	}
	return wrapErr(ErrSend, err.Error())
}

func recvError(err error) error {
	if isDisconnectError(err) {
		return ErrDisconnected
	}
	return wrapErr(ErrRecv, err.Error())
}

// overlappedRead reads one message and waits for it. A message larger than
// buf (ERROR_MORE_DATA) fails, as on a synchronous handle.
func overlappedRead(handle syscall.Handle, io *pipeIO, buf []byte) (int, error) {
	ov := &io.read
	io.readBuf = buf
	defer func() { io.readBuf = nil }()
	var read uint32
	// Overlapped I/O takes no byte-count pointer (Win32 docs); the count
	// comes from GetOverlappedResult.
	err := syscall.ReadFile(handle, buf, nil, ov)
	if err != nil && err != syscall.Errno(_ERROR_IO_PENDING) {
		return 0, recvError(err)
	}
	if err := getOverlappedResult(handle, ov, &read, true); err != nil {
		return 0, recvError(err)
	}
	if read == 0 {
		return 0, ErrDisconnected
	}
	return int(read), nil
}

// startOverlappedWrite posts one message. Its completion is collected with
// finishOverlappedWrite, also when WriteFile completed at once.
func startOverlappedWrite(handle syscall.Handle, io *pipeIO, data []byte) error {
	io.writeBuf = data
	// No byte-count pointer: the write may still be pending when this
	// returns; finishOverlappedWrite gets the count.
	err := syscall.WriteFile(handle, data, nil, &io.write)
	if err != nil && err != syscall.Errno(_ERROR_IO_PENDING) {
		io.writeBuf = nil
		return sendError(err)
	}
	return nil
}

func finishOverlappedWrite(handle syscall.Handle, io *pipeIO, data []byte) error {
	var written uint32
	err := getOverlappedResult(handle, &io.write, &written, true)
	io.writeBuf = nil
	if err != nil {
		return sendError(err)
	}
	if written != uint32(len(data)) { // #nosec G115 -- messages are bounded by the uint32 packet size.
		return wrapErr(ErrSend, fmt.Sprintf("short write: %d/%d", written, len(data)))
	}
	return nil
}

func overlappedWrite(handle syscall.Handle, io *pipeIO, data []byte) error {
	if err := startOverlappedWrite(handle, io, data); err != nil {
		return err
	}
	return finishOverlappedWrite(handle, io, data)
}

// writePacket writes one wire packet on the session's handle.
func (s *Session) writePacket(msg []byte) error {
	if s.io == nil {
		return rawWrite(s.handle, msg)
	}
	if s.progressBound == 0 {
		if s.sendBounded {
			return s.overlappedWriteWait(msg)
		}
		return overlappedWrite(s.handle, s.io, msg)
	}
	return s.writeWithProgress(msg)
}

// overlappedWriteWait posts the write and waits until it completes, the
// deadline passes or the abort channel is closed. A write that ends early is
// cancelled and the session broken, since part of the message may be on the
// wire.
func (s *Session) overlappedWriteWait(msg []byte) error {
	if err := startOverlappedWrite(s.handle, s.io, msg); err != nil {
		return err
	}
	ov := &s.io.write
	for {
		var written uint32
		err := getOverlappedResult(s.handle, ov, &written, false)
		if err == nil {
			s.io.writeBuf = nil
			if written != uint32(len(msg)) { // #nosec G115 -- messages are bounded by the uint32 packet size.
				return wrapErr(ErrSend, fmt.Sprintf("short write: %d/%d", written, len(msg)))
			}
			return nil
		}
		if err != syscall.Errno(_ERROR_IO_INCOMPLETE) {
			s.io.writeBuf = nil
			return sendError(err)
		}
		ms, werr := s.sendWait.waitMs()
		if werr != nil {
			s.cancelWrite()
			s.breakSession()
			return werr
		}
		if _, err := syscall.WaitForSingleObject(ov.HEvent, ms); err != nil {
			s.cancelWrite()
			s.breakSession()
			return wrapErr(ErrSend, fmt.Sprintf("WaitForSingleObject: %v", err))
		}
	}
}

// readPacket reads one wire packet from the session's handle.
func (s *Session) readPacket(dst []byte) (int, error) {
	if s.io == nil {
		return rawRecv(s.handle, dst)
	}
	return overlappedRead(s.handle, s.io, dst)
}

// writeWithProgress posts the write and, until it completes, reads what the
// server sent into the pending queue, so both sides keep making progress.
// Without sendWait it waits as long as a blocking write would; with one it
// ends at the deadline or abort.
func (s *Session) writeWithProgress(msg []byte) error {
	ov := &s.io.write
	if err := startOverlappedWrite(s.handle, s.io, msg); err != nil {
		return err
	}
	eof := false
	yieldUntil := monotime.After(progressYield)
	for {
		ev, werr := syscall.WaitForSingleObject(ov.HEvent, 0)
		if ev == _WAIT_OBJECT_0 {
			return finishOverlappedWrite(s.handle, s.io, msg)
		}
		if ev != _WAIT_TIMEOUT {
			s.cancelWrite()
			s.breakSession()
			return wrapErr(ErrSend, fmt.Sprintf("WaitForSingleObject: %v", werr))
		}
		if s.sendBounded {
			if _, err := s.sendWait.waitMs(); err != nil {
				s.cancelWrite()
				s.breakSession()
				return err
			}
		}
		if !eof {
			drained, derr := s.drainPending(&eof)
			if derr != nil {
				s.cancelWrite()
				s.breakSession()
				return derr
			}
			if drained {
				yieldUntil = monotime.After(progressYield)
				continue
			}
		}
		// The server is busy or its reply is still being written: yield for
		// up to progressYield after the last progress, then wait for the
		// write in short slices. A slice lasts a system timer tick and only
		// the write completing ends it early, so slicing at once stalls a
		// server blocked on its reply to us.
		if monotime.Now() < yieldUntil {
			procSwitchToThread.Call()
			continue
		}
		_, _ = syscall.WaitForSingleObject(ov.HEvent, 1)
	}
}

// cancelWrite cancels the posted write and waits until the kernel no longer
// uses its OVERLAPPED and buffer.
func (s *Session) cancelWrite() {
	ov := &s.io.write
	_ = syscall.CancelIoEx(s.handle, ov)
	var written uint32
	_ = getOverlappedResult(s.handle, ov, &written, true)
	s.io.writeBuf = nil
}

// drainPending reads every message available now into the pending queue.
// It reports whether it read any. A disconnected pipe sets *eof: the posted
// write reports it. Queued bytes beyond progressBound fail with
// ErrProtocol.
func (s *Session) drainPending(eof *bool) (bool, error) {
	drained := false
	readLen := int(s.PacketSize)
	for {
		available, err := peekNamedPipeAvailable(s.handle)
		if err != nil {
			if isDisconnectError(err) {
				*eof = true
				return drained, nil
			}
			// Stopping the drain on a live pipe would bring the deadlock back.
			return drained, wrapErr(ErrProtocol, "pending peek: "+err.Error())
		}
		if available == 0 {
			return drained, nil
		}
		q := &s.pending
		q.compact()
		start := len(q.buf)
		q.grow(pendingRecordLen + readLen)
		n, rerr := overlappedRead(s.handle, s.io, q.buf[start+pendingRecordLen:])
		if rerr != nil {
			q.buf = q.buf[:start]
			if rerr == ErrDisconnected {
				*eof = true
				return drained, nil
			}
			// A message larger than a packet, or a read failure.
			return drained, wrapErr(ErrProtocol, "pending read: "+rerr.Error())
		}
		binary.NativeEndian.PutUint32(q.buf[start:], uint32(n)) // #nosec G115 -- n <= readLen, a uint32 packet size.
		q.buf = q.buf[:start+pendingRecordLen+n]
		if len(q.buf)-q.head > s.progressBound {
			return drained, wrapErr(ErrProtocol, "inbound bytes exceed the in-flight responses")
		}
		drained = true
	}
}

// breakSession closes the pipe: a cancelled write leaves the message stream
// in an unknown state, and a drain failure drops inbound messages. Later
// calls fail with ErrBadParam and Close still releases the rest.
func (s *Session) breakSession() {
	if s.handle != syscall.InvalidHandle {
		syscall.CloseHandle(s.handle)
		s.handle = syscall.InvalidHandle
	}
	s.pending = pendingQueue{}
	s.failAllInflight()
}

// pendingRecordLen is the length prefix of each queued message.
const pendingRecordLen = 4

// pendingQueue holds messages a pipelining client send read while its write
// waited; receive returns them before reading the pipe.
type pendingQueue struct {
	buf  []byte
	head int
}

func (q *pendingQueue) queued() bool {
	return q.head < len(q.buf)
}

// grow extends buf by n bytes. It zeroes only newly allocated storage, not
// the bytes a read will overwrite.
func (q *pendingQueue) grow(n int) {
	need := len(q.buf) + n
	if need > cap(q.buf) {
		nb := make([]byte, len(q.buf), max(need, 2*cap(q.buf)))
		copy(nb, q.buf)
		q.buf = nb
	}
	q.buf = q.buf[:need]
}

func (q *pendingQueue) compact() {
	if q.head > 0 {
		n := copy(q.buf, q.buf[q.head:])
		q.buf = q.buf[:n]
		q.head = 0
	}
}

// pop copies the oldest queued message into dst. A message larger than dst
// fails, as a pipe read would with ERROR_MORE_DATA.
func (q *pendingQueue) pop(dst []byte) (int, error) {
	rec := int(binary.NativeEndian.Uint32(q.buf[q.head:]))
	start := q.head + pendingRecordLen
	var n int
	var err error
	if rec > len(dst) {
		err = wrapErr(ErrRecv, fmt.Sprintf("queued message of %d bytes exceeds the %d-byte buffer", rec, len(dst)))
	} else {
		n = copy(dst, q.buf[start:start+rec])
	}
	q.head = start + rec
	if q.head == len(q.buf) {
		q.buf = q.buf[:0]
		q.head = 0
	}
	return n, err
}

// pendingBound is the wire bytes inflight responses can occupy: the pending
// queue bound. It saturates at math.MaxInt (32-bit targets), as on POSIX.
func pendingBound(maxResponsePayload, packetSize uint32, inflight int) int {
	payload := uint64(maxResponsePayload)
	budget := max(uint64(packetSize), protocol.HeaderSize+1) - protocol.HeaderSize
	packets := uint64(1)
	if payload > 0 {
		packets = 1 + (payload-1)/budget
	}
	perResponse := payload + packets*uint64(protocol.HeaderSize+pendingRecordLen)
	if perResponse > uint64(math.MaxInt)/uint64(inflight) {
		return math.MaxInt
	}
	return int(perResponse * uint64(inflight)) // #nosec G115 -- checked against math.MaxInt above.
}
