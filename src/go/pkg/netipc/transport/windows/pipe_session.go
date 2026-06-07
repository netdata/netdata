//go:build windows

package windows

import (
	"syscall"
	"time"
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
}

func (s *Session) failAllInflight() {
	if s.role != RoleClient || len(s.inflightIDs) == 0 {
		return
	}
	clear(s.inflightIDs)
}

// Handle returns the raw HANDLE for WaitForSingleObject integration.
func (s *Session) Handle() syscall.Handle {
	return s.handle
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

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
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
		if !time.Now().Before(deadline) {
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
				if !time.Now().Before(deadline) {
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
		if s.role == RoleServer {
			flushFileBuffers(s.handle)
			disconnectNamedPipe(s.handle)
		}
		syscall.CloseHandle(s.handle)
		s.handle = syscall.InvalidHandle
	}
	s.recvBuf = nil
	s.sendBuf = nil
	s.pktBuf = nil
	s.failAllInflight()
}

// Connect establishes a session to a server pipe derived from runDir + serviceName.
func Connect(runDir, serviceName string, config *ClientConfig) (*Session, error) {
	pipeName, err := BuildPipeName(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		&pipeName[0],
		_GENERIC_READ|_GENERIC_WRITE,
		0,
		nil,
		_OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, wrapErr(ErrConnect, err.Error())
	}

	mode := uint32(_PIPE_READMODE_MESSAGE)
	if err := setNamedPipeHandleState(handle, &mode); err != nil {
		syscall.CloseHandle(handle)
		return nil, wrapErr(ErrConnect, "SetNamedPipeHandleState: "+err.Error())
	}

	session, herr := clientHandshake(handle, config)
	if herr != nil {
		syscall.CloseHandle(handle)
		return nil, herr
	}
	return session, nil
}
