//go:build unix

package posix

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
)

// Listener is a listening UDS SEQPACKET endpoint.
type Listener struct {
	mu            sync.Mutex
	fd            int
	config        ServerConfig
	path          string
	nextSessionID atomic.Uint64
}

// Listen creates a listener on {runDir}/{serviceName}.sock.
// Performs stale endpoint recovery.
func Listen(runDir, serviceName string, config ServerConfig) (*Listener, error) {
	path, err := buildSocketPath(runDir, serviceName)
	if err != nil {
		return nil, err
	}

	stale := checkAndRecoverStale(path)
	if stale == staleLiveServer {
		return nil, ErrAddrInUse
	}

	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, wrapErr(ErrSocket, err.Error())
	}

	sa := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Bind(fd, sa); err != nil {
		_ = syscall.Close(fd)
		return nil, wrapErr(ErrSocket, "bind: "+err.Error())
	}

	backlog := config.Backlog
	if backlog <= 0 {
		backlog = defaultBacklog
	}

	if err := syscall.Listen(fd, backlog); err != nil {
		_ = syscall.Close(fd)
		_ = os.Remove(path)
		return nil, wrapErr(ErrSocket, "listen: "+err.Error())
	}

	return &Listener{
		fd:     fd,
		config: config,
		path:   path,
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
// per-session server config and session ID.
func (l *Listener) AcceptWithConfig(sessionID uint64, config ServerConfig) (*Session, error) {
	l.mu.Lock()
	fd := l.fd
	l.mu.Unlock()
	if fd < 0 {
		return nil, wrapErr(ErrAccept, "listener closed")
	}

	nfd, _, err := syscall.Accept(fd)
	if err != nil {
		return nil, wrapErr(ErrAccept, err.Error())
	}

	session, herr := serverHandshake(nfd, &config, sessionID)
	if herr != nil {
		_ = syscall.Close(nfd)
		return nil, herr
	}
	return session, nil
}

// Close closes the listener, stops accepting, and unlinks the socket file.
func (l *Listener) Close() {
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
