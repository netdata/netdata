//go:build windows

package windows

import (
	"sync"
	"sync/atomic"
	"syscall"
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
// per-session server config and session ID.
func (l *Listener) AcceptWithConfig(sessionID uint64, config ServerConfig) (*Session, error) {
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
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, wrapErr(ErrAccept, "listener closed")
	}

	bufSize := pipeBufferSize(l.config.PacketSize)
	next, perr := createPipeInstance(l.pipeName, bufSize, false)
	if perr != nil {
		if l.handle == sessionHandle {
			l.handle = syscall.InvalidHandle
		}
		l.mu.Unlock()
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, perr
	}
	l.handle = next
	l.mu.Unlock()

	session, herr := serverHandshake(sessionHandle, &config, sessionID)
	if herr != nil {
		disconnectNamedPipe(sessionHandle)
		syscall.CloseHandle(sessionHandle)
		return nil, herr
	}
	return session, nil
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
			0,
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

	handle, err := createNamedPipe(
		&pipeName[0],
		openMode,
		_PIPE_TYPE_MESSAGE|_PIPE_READMODE_MESSAGE|_PIPE_WAIT,
		_PIPE_UNLIMITED_INSTANCES,
		bufSize,
		bufSize,
		0,
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
