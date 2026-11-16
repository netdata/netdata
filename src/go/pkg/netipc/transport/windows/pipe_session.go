//go:build windows

package windows

import (
	"errors"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
)

// Role distinguishes client vs server sessions.
type Role int

const (
	RoleClient Role = 1
	RoleServer Role = 2
)

// spinWaitIterations limits cooperative polling before falling back to sleep.
const spinWaitIterations = 256

// ClientConfig configures a client connection.
type ClientConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32 // 0 = use default (1024)
	MaxRequestBatchItems    uint32 // 0 = use default (1)
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32 // 0 = use default (65536)
}

// ServerConfig configures a listener and its accepted sessions.
type ServerConfig struct {
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32 // 0 = use default (65536)
}

// Session is a connected Named Pipe session (client or server side).
type Session struct {
	handle syscall.Handle
	role   Role

	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	PacketSize              uint32
	SelectedProfile         uint32
	SessionID               uint64

	recvBuf []byte
	sendBuf []byte
	pktBuf  []byte

	inflightIDs map[uint64]struct{}

	// Overlapped state of a client handle; nil on server sessions, whose
	// handles are synchronous.
	io *pipeIO
	// Messages read while a pipelined send waited for its write.
	pending pendingQueue
	// Queue bound of the send in progress; 0 writes without draining.
	progressBound int
	// sendWait bounds the send in progress when sendBounded is set;
	// otherwise the send waits without a deadline.
	sendWait    receiveWait
	sendBounded bool

	// Receive state of the call in progress, read by closures built once
	// per session (initReceiveFns), so a receive allocates nothing.
	recvWait      receiveWait
	recvControl   bool
	recvPacketFn  func([]byte) (int, error)
	failInflights func()
}

func (s *Session) failAllInflight() {
	if s.role != RoleClient || len(s.inflightIDs) == 0 {
		return
	}
	clear(s.inflightIDs)
}

// Handle returns the raw pipe HANDLE. It is not a readability signal:
// a pipe handle is signalled by I/O completion, not by arriving data. Use
// WaitReadable, and check HasPending first: a pipelined send can read
// responses into the session, where the pipe no longer shows them.
func (s *Session) Handle() syscall.Handle {
	return s.handle
}

// HasPending reports whether the next Receive can return a message read
// earlier by a pipelined send, without the pipe becoming readable.
func (s *Session) HasPending() bool {
	return s.pending.queued()
}

// Role returns the session role.
func (s *Session) Role() Role {
	return s.role
}

// GetRole returns the session role.
// Deprecated: use Role.
func (s *Session) GetRole() Role {
	return s.Role()
}

// WaitReadable waits until bytes are available to read or the timeout expires.
func (s *Session) WaitReadable(timeoutMs uint32) (bool, error) {
	if s.handle == syscall.InvalidHandle {
		return false, wrapErr(ErrBadParam, "session closed")
	}
	if s.pending.queued() {
		return true, nil
	}

	deadline := monotime.After(time.Duration(timeoutMs) * time.Millisecond)
	yielded := false
	for {
		available, err := peekNamedPipeAvailable(s.handle)
		if err != nil {
			if isDisconnectError(err) {
				s.failAllInflight()
				return false, ErrDisconnected
			}
			return false, wrapErr(ErrRecv, err.Error())
		}
		if available > 0 {
			return true, nil
		}
		if monotime.Now() >= deadline {
			return false, nil
		}
		if !yielded {
			yielded = true
			for i := 0; i < spinWaitIterations; i++ {
				procSwitchToThread.Call()
				available, err = peekNamedPipeAvailable(s.handle)
				if err != nil {
					if isDisconnectError(err) {
						s.failAllInflight()
						return false, ErrDisconnected
					}
					return false, wrapErr(ErrRecv, err.Error())
				}
				if available > 0 {
					return true, nil
				}
				if monotime.Now() >= deadline {
					return false, nil
				}
			}
			continue
		}
		time.Sleep(time.Millisecond)
	}
}

// Close closes the session and releases resources.
func (s *Session) Close() {
	if s.handle != syscall.InvalidHandle {
		// Close only. The peer still reads every message written before the
		// close, then sees ERROR_BROKEN_PIPE. FlushFileBuffers would block on
		// a peer that stopped reading, and neither CancelSynchronousIo nor
		// DisconnectNamedPipe from another thread releases it.
		syscall.CloseHandle(s.handle)
		s.handle = syscall.InvalidHandle
	}
	// No I/O stays posted between calls, so the events can go.
	s.io.close()
	s.io = nil
	s.pending = pendingQueue{}
	s.recvBuf = nil
	s.sendBuf = nil
	s.pktBuf = nil
	s.failAllInflight()
}

// ConnectBusyWaitMs bounds how long Connect waits for a free pipe instance
// when every instance is busy (ERROR_PIPE_BUSY).
const ConnectBusyWaitMs = 1000

// connectBusyHook, when set by tests, runs whenever a connect finds every
// pipe instance busy, before it waits.
var connectBusyHook func()

// connectWaitedHook, when set by tests, runs whenever waitNamedPipe reports a
// free instance, before the connect retries, so a test can take it first.
var connectWaitedHook func()

// createClientPipe opens with SECURITY_IDENTIFICATION, so the server can
// identify the client but never act as it, and overlapped, so a pipelining
// send can read while its write waits.
func createClientPipe(name *uint16) (syscall.Handle, error) {
	return syscall.CreateFile(name, _GENERIC_READ|_GENERIC_WRITE, 0, nil, _OPEN_EXISTING,
		_SECURITY_SQOS_PRESENT|_SECURITY_IDENTIFICATION|_FILE_FLAG_OVERLAPPED, 0)
}

// connectBusySliceMs is the longest single busy-pipe wait, so an abort ends
// it promptly.
const connectBusySliceMs = 100

// openClientPipe opens the client end of a pipe. A server has no free pipe
// instance between accepting one client and creating the next instance, and
// connects in that window fail with ERROR_PIPE_BUSY. Wait, bounded by
// ConnectBusyWaitMs and the caller's deadline, for an instance and retry:
// waitNamedPipe does not reserve it, so another client may take it first.
func openClientPipe(name *uint16, wait receiveWait) (syscall.Handle, error) {
	if _, werr := wait.waitMs(); werr != nil {
		return syscall.InvalidHandle, werr
	}
	handle, err := createClientPipe(name)
	if errno, ok := err.(syscall.Errno); !ok || errno != _ERROR_PIPE_BUSY {
		if err != nil {
			return handle, wrapErr(ErrConnect, err.Error())
		}
		return handle, nil
	}
	if connectBusyHook != nil {
		connectBusyHook()
	}

	busyDeadline := monotime.After(ConnectBusyWaitMs * time.Millisecond)
	if !wait.infinite && wait.deadline < busyDeadline {
		busyDeadline = wait.deadline
	}
	for {
		if _, werr := wait.waitMs(); errors.Is(werr, ErrAborted) {
			return syscall.InvalidHandle, werr
		}
		now := monotime.Now()
		left := time.Duration(busyDeadline - now)
		if left <= 0 {
			if !wait.infinite && now >= wait.deadline {
				return syscall.InvalidHandle, ErrTimeout
			}
			return syscall.InvalidHandle, wrapErr(ErrConnect, err.Error())
		}
		slice := boundedReceiveWaitMs(left, connectBusySliceMs)
		werr := waitNamedPipe(name, slice)
		if werr == nil {
			if connectWaitedHook != nil {
				connectWaitedHook()
			}
			handle, err = createClientPipe(name)
			if err == nil {
				return handle, nil
			}
			if errno, ok := err.(syscall.Errno); !ok || errno != _ERROR_PIPE_BUSY {
				return handle, wrapErr(ErrConnect, err.Error())
			}
			// Another client took the instance first.
			continue
		}
		if errno, ok := werr.(syscall.Errno); !ok || errno != _ERROR_SEM_TIMEOUT {
			return syscall.InvalidHandle, wrapErr(ErrConnect, werr.Error())
		}
	}
}

// Connect establishes a session to a server pipe derived from runDir + serviceName.
// When every pipe instance is busy it waits up to ConnectBusyWaitMs for one.
func Connect(runDir, serviceName string, config *ClientConfig) (*Session, error) {
	return ConnectTimeout(runDir, serviceName, config, 0, nil)
}

// ConnectTimeout is Connect bounded by timeoutMs: the busy-pipe wait and the
// wait for HELLO_ACK together end with ErrTimeout after timeoutMs (0 = no
// deadline), and with ErrAborted once abortCh is closed (nil = no abort).
func ConnectTimeout(runDir, serviceName string, config *ClientConfig, timeoutMs uint32, abortCh <-chan struct{}) (*Session, error) {
	wait := newReceiveWait(timeoutMs, abortCh)
	pipeName, err := BuildPipeName(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	handle, err := openClientPipe(&pipeName[0], wait)
	if err != nil {
		return nil, err
	}

	// Whoever created the pipe name first owns it. Send HELLO and the auth
	// token only to a server running as this account, SYSTEM or
	// Administrators; other accounts cannot take those as owner.
	if !ownerTrusted(handle) {
		syscall.CloseHandle(handle)
		return nil, ErrUntrustedServer
	}

	mode := uint32(_PIPE_READMODE_MESSAGE)
	if err := setNamedPipeHandleState(handle, &mode); err != nil {
		syscall.CloseHandle(handle)
		return nil, wrapErr(ErrConnect, "SetNamedPipeHandleState: "+err.Error())
	}

	io, err := newPipeIO()
	if err != nil {
		syscall.CloseHandle(handle)
		return nil, err
	}
	session, herr := clientHandshake(handle, io, config, wait)
	if herr != nil {
		syscall.CloseHandle(handle)
		io.close()
		return nil, herr
	}
	return session, nil
}
