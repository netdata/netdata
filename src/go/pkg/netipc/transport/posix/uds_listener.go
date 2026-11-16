//go:build unix

package posix

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Listener is a listening UDS endpoint.
type Listener struct {
	mu            sync.Mutex
	stopMu        sync.Mutex
	fd            int
	config        ServerConfig
	path          string
	nextSessionID atomic.Uint64
	// lock holds {serviceName}.lock flock'ed until Close.
	lock *os.File
}

// listenHook, when set by tests, runs inside Listen between bind() and
// listen(), with the socket path.
var listenHook atomic.Pointer[func(path string)]

// Listen creates a listener on {runDir}/{serviceName}.sock.
// It first takes an exclusive flock on {runDir}/{serviceName}.lock, held
// until Close; a held lock means another server owns the endpoint
// (ErrAddrInUse). With the lock held, an existing socket file no live
// server accepts on is stale and is replaced.
func Listen(runDir, serviceName string, config ServerConfig) (*Listener, error) {
	path, err := buildSocketPath(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	lock, err := acquireEndpointLock(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	// The probe still guards against a live server that takes no lock (an
	// older library version).
	stale := checkAndRecoverStale(path)
	if stale == staleLiveServer {
		releaseEndpointLock(lock)
		return nil, ErrAddrInUse
	}

	fd, err := udsSocket()
	if err != nil {
		releaseEndpointLock(lock)
		return nil, wrapErr(ErrSocket, err.Error())
	}

	if err := bindOwnerOnly(fd, runDir, serviceName, path); err != nil {
		_ = syscall.Close(fd)
		releaseEndpointLock(lock)
		return nil, err
	}

	if hook := listenHook.Load(); hook != nil {
		(*hook)(path)
	}

	backlog := config.Backlog
	if backlog <= 0 {
		backlog = defaultBacklog
	}

	if err := syscall.Listen(fd, backlog); err != nil {
		_ = syscall.Close(fd)
		_ = os.Remove(path)
		releaseEndpointLock(lock)
		return nil, wrapErr(ErrSocket, "listen: "+err.Error())
	}
	if err := prepareListener(fd); err != nil {
		_ = syscall.Close(fd)
		_ = os.Remove(path)
		releaseEndpointLock(lock)
		return nil, wrapErr(ErrSocket, "listener flags: "+err.Error())
	}

	return &Listener{
		fd:     fd,
		config: config,
		path:   path,
		lock:   lock,
	}, nil
}

// Fd returns the raw file descriptor for poll/epoll integration.
func (l *Listener) Fd() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fd
}

// SetPayloadLimits updates the payload limits used for future handshakes.
func (l *Listener) SetPayloadLimits(maxRequestPayloadBytes, maxResponsePayloadBytes uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.MaxRequestPayloadBytes = maxRequestPayloadBytes
	l.config.MaxResponsePayloadBytes = maxResponsePayloadBytes
}

// Accept accepts one client connection. Performs the full handshake.
// Blocks until a client connects and the handshake completes.
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

// PendingConn is an accepted connection whose handshake has not run yet.
// Splitting accept from handshake lets a server bound the handshake and run
// it outside its accept loop.
type PendingConn struct {
	fd int
}

// AcceptPending accepts one connection without reading HELLO.
func (l *Listener) AcceptPending() (*PendingConn, error) {
	l.mu.Lock()
	fd := l.fd
	l.mu.Unlock()
	if fd < 0 {
		return nil, wrapErr(ErrAccept, "listener closed")
	}

	nfd, err := acceptCloexec(fd)
	if err != nil {
		return nil, wrapErr(ErrAccept, err.Error())
	}
	return &PendingConn{fd: nfd}, nil
}

// Fd returns the connection's file descriptor, or -1 once consumed.
func (p *PendingConn) Fd() int {
	return p.fd
}

// Handshake reads HELLO and answers it. timeoutMs bounds the wait for HELLO
// (0 = no deadline) and abortCh, when non-nil, cancels it. The connection is
// consumed: on success the Session owns the fd, on failure it is closed.
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
	fd := p.fd
	if fd < 0 {
		return nil, wrapErr(ErrAccept, "connection already consumed")
	}
	p.fd = -1

	session, err := serverHandshakeWait(fd, &config, sessionID, newReceiveWait(timeoutMs, abortCh), prepare)
	if err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return session, nil
}

// Close drops a connection that will not be handshaken.
func (p *PendingConn) Close() {
	if p.fd >= 0 {
		_ = syscall.Close(p.fd)
		p.fd = -1
	}
}

// Close stops listening if it still is, then releases the endpoint lock.
func (l *Listener) Close() {
	l.StopListening()

	l.mu.Lock()
	lock := l.lock
	l.lock = nil
	l.mu.Unlock()
	// Release the endpoint only after its socket file is gone.
	if lock != nil {
		releaseEndpointLock(lock)
	}
}

// StopListening closes the listening socket and removes the socket file,
// keeping the endpoint lock until Close, so no other server takes the
// endpoint while this one still serves its sessions. Idempotent.
func (l *Listener) StopListening() {
	// A concurrent caller waits for the unlink, so Close cannot release the
	// endpoint while another Close is still removing the socket file.
	l.stopMu.Lock()
	defer l.stopMu.Unlock()

	l.mu.Lock()
	fd := l.fd
	path := l.path
	l.fd = -1
	l.path = ""
	l.mu.Unlock()

	if fd >= 0 {
		_ = syscall.Close(fd)
	}
	if path != "" {
		_ = os.Remove(path)
	}
}
