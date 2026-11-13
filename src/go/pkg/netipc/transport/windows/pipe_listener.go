//go:build windows

package windows

import (
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Listener is a listening Named Pipe endpoint.
type Listener struct {
	mu            sync.Mutex
	handle        syscall.Handle
	config        ServerConfig
	pipeName      []uint16
	nextSessionID atomic.Uint64
	closing       bool
	accepting     bool
}

// Listen creates a listener on a Named Pipe derived from runDir + serviceName.
func Listen(runDir, serviceName string, config ServerConfig) (*Listener, error) {
	pipeName, err := BuildPipeName(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	bufSize := pipeBufferSize(config.PacketSize)
	handle, err := createPipeInstance(pipeName, bufSize, true)
	if err != nil {
		return nil, err
	}

	return &Listener{
		handle:   handle,
		config:   config,
		pipeName: pipeName,
	}, nil
}

// Handle returns the raw HANDLE.
func (l *Listener) Handle() syscall.Handle {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.handle
}

// SetPayloadLimits updates the payload limits used for future handshakes.
func (l *Listener) SetPayloadLimits(maxRequestPayloadBytes, maxResponsePayloadBytes uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.MaxRequestPayloadBytes = maxRequestPayloadBytes
	l.config.MaxResponsePayloadBytes = maxResponsePayloadBytes
}

// Accept accepts one client connection. Performs the full handshake.
func (l *Listener) Accept() (*Session, error) {
	sessionID := l.nextSessionID.Add(1)
	l.mu.Lock()
	config := l.config
	l.mu.Unlock()
	return l.AcceptWithConfig(sessionID, config)
}

// AcceptWithConfig accepts one client connection using a caller-provided
// per-session server config and session ID. The handshake waits at most
// protocol.ServerHandshakeTimeoutMs for HELLO; servers that must not block
// their accept loop use AcceptPending and PendingConn.Handshake instead.
func (l *Listener) AcceptWithConfig(sessionID uint64, config ServerConfig) (*Session, error) {
	pending, err := l.AcceptPending()
	if err != nil {
		return nil, err
	}
	return pending.Handshake(sessionID, config, protocol.ServerHandshakeTimeoutMs, nil)
}

// PendingConn is a connected pipe instance whose handshake has not run yet.
// Splitting accept from handshake lets a server bound the handshake and run
// it outside its accept loop.
type PendingConn struct {
	handle syscall.Handle
}

// failNextInstances, when set by tests, fails that many creations of a
// replacement pipe instance in AcceptPending, as CreateNamedPipeW can under
// resource exhaustion.
var failNextInstances atomic.Int32

// AcceptPending waits for one client connection without reading HELLO.
func (l *Listener) AcceptPending() (*PendingConn, error) {
	l.mu.Lock()
	if l.handle == syscall.InvalidHandle {
		l.mu.Unlock()
		return nil, wrapErr(ErrAccept, "listener closed")
	}
	sessionHandle := l.handle
	l.accepting = true
	l.mu.Unlock()

	err := connectNamedPipe(sessionHandle)
	if err != nil {
		if errno, ok := err.(syscall.Errno); !ok || errno != _ERROR_PIPE_CONNECTED {
			l.mu.Lock()
			l.accepting = false
			l.mu.Unlock()
			return nil, wrapErr(ErrAccept, err.Error())
		}
	}

	l.mu.Lock()
	l.accepting = false
	if l.closing {
		if l.handle == sessionHandle {
			l.handle = syscall.InvalidHandle
		}
		l.mu.Unlock()
		syscall.CloseHandle(sessionHandle)
		return nil, wrapErr(ErrAccept, "listener closed")
	}

	bufSize := pipeBufferSize(l.config.PacketSize)
	var next syscall.Handle
	var perr error
	if failNextInstances.Load() > 0 && failNextInstances.Add(-1) >= 0 {
		perr = wrapErr(ErrCreatePipe, "test: replacement instance failed")
	} else {
		next, perr = createPipeInstance(l.pipeName, bufSize, false)
	}
	if perr != nil {
		// Without a next instance, drop this client and keep its instance as
		// the listener's, so a later accept listens on it again.
		disconnectNamedPipe(sessionHandle)
		l.mu.Unlock()
		return nil, perr
	}
	l.handle = next
	l.mu.Unlock()

	return &PendingConn{handle: sessionHandle}, nil
}

// Handle returns the pipe handle, or InvalidHandle once consumed.
func (p *PendingConn) Handle() syscall.Handle {
	return p.handle
}

// Handshake reads HELLO and answers it. timeoutMs bounds the wait for HELLO
// (0 = no deadline) and abortCh, when non-nil, cancels it. The connection is
// consumed: on success the Session owns the handle, on failure it is closed.
func (p *PendingConn) Handshake(
	sessionID uint64,
	config ServerConfig,
	timeoutMs uint32,
	abortCh <-chan struct{},
) (*Session, error) {
	return p.HandshakePrepared(sessionID, config, timeoutMs, abortCh, nil)
}

// ProfilePrepare sets up a selected profile's resources (an SHM region)
// during the server handshake, once the payload limits are agreed and
// before HELLO_ACK is sent. Returning false drops that profile and the
// handshake selects again; with no profile left the client is rejected as
// unsupported.
type ProfilePrepare func(profile, maxRequestPayloadBytes, maxResponsePayloadBytes uint32) bool

// HandshakePrepared is Handshake with a prepare step; prepare may be nil.
func (p *PendingConn) HandshakePrepared(
	sessionID uint64,
	config ServerConfig,
	timeoutMs uint32,
	abortCh <-chan struct{},
	prepare ProfilePrepare,
) (*Session, error) {
	handle := p.handle
	if handle == syscall.InvalidHandle {
		return nil, wrapErr(ErrAccept, "connection already consumed")
	}
	p.handle = syscall.InvalidHandle

	session, err := serverHandshakeWait(handle, &config, sessionID, newReceiveWait(timeoutMs, abortCh), prepare)
	if err != nil {
		// Close only: DisconnectNamedPipe would discard a rejection reply
		// the client has not read yet.
		syscall.CloseHandle(handle)
		return nil, err
	}
	return session, nil
}

// Close drops a connection that will not be handshaken.
func (p *PendingConn) Close() {
	if p.handle != syscall.InvalidHandle {
		syscall.CloseHandle(p.handle)
		p.handle = syscall.InvalidHandle
	}
}

// Wake connects to the listener once and disconnects, so an accept blocked
// in ConnectNamedPipe returns. It never closes the listener's handle.
func (l *Listener) Wake() {
	l.mu.Lock()
	pipeName := l.pipeName
	l.mu.Unlock()
	if len(pipeName) == 0 || pipeName[0] == 0 {
		return
	}

	wake, err := syscall.CreateFile(
		&pipeName[0],
		_GENERIC_READ|_GENERIC_WRITE,
		0,
		nil,
		_OPEN_EXISTING,
		_SECURITY_SQOS_PRESENT|_SECURITY_IDENTIFICATION,
		0,
	)
	if err == nil && wake != syscall.InvalidHandle && wake != 0 {
		syscall.CloseHandle(wake)
	}
}

// Close closes the listener.
func (l *Listener) Close() {
	l.mu.Lock()
	handle := l.handle
	if handle == syscall.InvalidHandle {
		l.mu.Unlock()
		return
	}
	l.closing = true
	accepting := l.accepting
	if !accepting {
		l.handle = syscall.InvalidHandle
	}
	pipeName := l.pipeName
	l.mu.Unlock()

	if accepting && len(pipeName) > 0 && pipeName[0] != 0 {
		wake, err := syscall.CreateFile(
			&pipeName[0],
			_GENERIC_READ|_GENERIC_WRITE,
			0,
			nil,
			_OPEN_EXISTING,
			_SECURITY_SQOS_PRESENT|_SECURITY_IDENTIFICATION,
			0,
		)
		if err == nil && wake != syscall.InvalidHandle && wake != 0 {
			syscall.CloseHandle(wake)
			return
		}
		l.mu.Lock()
		if l.handle == handle {
			l.handle = syscall.InvalidHandle
		}
		l.mu.Unlock()
	}

	syscall.CloseHandle(handle)
}

func createPipeInstance(pipeName []uint16, bufSize uint32, firstInstance bool) (syscall.Handle, error) {
	openMode := uint32(_PIPE_ACCESS_DUPLEX)
	if firstInstance {
		openMode |= _FILE_FLAG_FIRST_PIPE_INSTANCE
	}

	attrs, err := newOwnerOnlyAttributes()
	if err != nil {
		return syscall.InvalidHandle, wrapErr(ErrCreatePipe, err.Error())
	}
	defer attrs.free()

	handle, err := createNamedPipe(
		&pipeName[0],
		openMode,
		_PIPE_TYPE_MESSAGE|_PIPE_READMODE_MESSAGE|_PIPE_WAIT|_PIPE_REJECT_REMOTE_CLIENTS,
		_PIPE_UNLIMITED_INSTANCES,
		bufSize,
		bufSize,
		0,
		attrs.ptr(),
	)
	if err != nil {
		errno, ok := err.(syscall.Errno)
		if ok && (errno == _ERROR_ACCESS_DENIED || errno == _ERROR_PIPE_BUSY) {
			return syscall.InvalidHandle, ErrAddrInUse
		}
		return syscall.InvalidHandle, wrapErr(ErrCreatePipe, err.Error())
	}
	return handle, nil
}
