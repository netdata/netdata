//go:build windows

package raw

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// Server is an internal managed server bound to one expected request kind.
type Server struct {
	runDir                       string
	serviceName                  string
	config                       windows.ServerConfig
	expectedMethodCode           uint16
	handler                      DispatchHandler
	running                      atomic.Bool
	learnedRequestPayloadBytes   atomic.Uint32
	learnedResponsePayloadBytes  atomic.Uint32
	requestPayloadGrowthCeiling  uint32
	responsePayloadGrowthCeiling uint32
	nextSessionID                atomic.Uint64
	workerCount                  int
	wg                           sync.WaitGroup
	listenerMu                   sync.Mutex
	listener                     *windows.Listener // stored so Stop() can wake it
	stopRequested                atomic.Bool
	stopInit                     sync.Once
	stopClose                    sync.Once
	stopCh                       chan struct{}
	timings                      serverTimings
	listening                    chan struct{} // closed once the listener accepts
	listeningOnce                sync.Once
	sessionExited                chan struct{} // test hook, nil in production: signalled after a worker frees its slot
	handshakeStarted             chan struct{} // test hook, nil in production: signalled before a worker waits for HELLO
	shmCreated                   chan struct{} // test hook, nil in production: signalled once a session's SHM objects exist
	failShmCreate                atomic.Bool   // test hook: makes SHM object creation fail
	workerThreadsMu              sync.Mutex
	workerThreads                map[syscall.Handle]struct{}
}

// NewServer creates a new managed server.
func NewServer(
	runDir, serviceName string,
	config windows.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *Server {
	return NewServerWithWorkers(runDir, serviceName, config, expectedMethodCode, handler, defaultServerWorkerCount)
}

// NewServerWithWorkers creates a server with an explicit worker count limit.
func NewServerWithWorkers(
	runDir, serviceName string,
	config windows.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
	workerCount int,
) *Server {
	s := &Server{config: config}
	s.initCommon(
		runDir,
		serviceName,
		expectedMethodCode,
		handler,
		workerCount,
		config.MaxRequestPayloadBytes,
		config.MaxResponsePayloadBytes,
	)
	return s
}

const winShmProfiles = windows.WinShmProfileHybrid | windows.WinShmProfileBusywait

func (s *Server) prepareAcceptConfig() (uint64, windows.ServerConfig) {
	sessionID := s.nextSessionID.Add(1)
	cfg := s.config
	cfg.MaxRequestPayloadBytes = s.learnedRequestPayloadBytes.Load()
	cfg.MaxResponsePayloadBytes = s.learnedResponsePayloadBytes.Load()
	return sessionID, cfg
}

// createShm creates a session's SHM objects for one profile and the agreed
// payload limits.
func (s *Server) createShm(sessionID uint64, profile, requestPayload, responsePayload uint32) *windows.WinShmContext {
	if s.failShmCreate.Load() {
		return nil
	}
	reqCap, reqOK := headerPayloadLen(requestPayload)
	respCap, respOK := headerPayloadLen(responsePayload)
	if !reqOK || !respOK {
		return nil
	}
	shm, err := windows.WinShmServerCreate(
		s.runDir, s.serviceName, s.config.AuthToken, sessionID, profile, reqCap, respCap)
	if err != nil {
		return nil
	}
	s.notifyTestHook(s.shmCreated)
	return shm
}

// Run starts the acceptor loop. Blocking. Accepts clients and runs each
// connection, handshake included, in its own goroutine (up to workerCount
// concurrently). The server is single-use: a Stop before Run makes Run
// return at once.
func (s *Server) Run() error {
	if s.stopRequested.Load() {
		return nil
	}

	listener, err := windows.Listen(s.runDir, s.serviceName, s.config)
	if err != nil {
		return err
	}
	s.setListener(listener)

	s.running.Store(true)
	// Stop may have run before the listener was published, so it could not
	// wake the accept below.
	if s.stopRequested.Load() {
		s.running.Store(false)
	}
	s.markListening()
	sem := make(chan struct{}, s.workerCount)

	for s.running.Load() {
		pending, err := listener.AcceptPending()
		if err != nil {
			if !s.retryAcceptAfter(nil) {
				break
			}
			continue
		}

		// The stop wake connection, or a client racing the stop.
		if !s.running.Load() {
			pending.Close()
			break
		}

		// Over capacity: close before HELLO_ACK so the client sees a
		// failed connect instead of a session that dies on first use.
		if !s.acquireWorkerSlot(sem, nil, pending.Close) {
			continue
		}

		s.startSessionWorker(sem, func() {
			s.serveConnection(pending)
		})
	}

	s.markStopped()
	s.setListener(nil)
	listener.Close()
	s.stopSessionWorkers()
	return nil
}

// Stop signals the server to stop and wakes its accept. Run owns the
// listener and closes it on exit, so Stop never closes a handle Run may
// still be using.
func (s *Server) Stop() {
	s.markStopped()

	s.listenerMu.Lock()
	listener := s.listener
	s.listenerMu.Unlock()
	if listener == nil {
		return
	}
	// One wake can be lost if it lands between accepts; repeat until Run
	// unpublishes the listener.
	go func() {
		for {
			listener.Wake()
			time.Sleep(serverStopRetry)
			s.listenerMu.Lock()
			current := s.listener
			s.listenerMu.Unlock()
			if current != listener {
				return
			}
		}
	}()
}

func (s *Server) setListener(listener *windows.Listener) {
	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()
}

func (s *Server) serveConnection(pending *windows.PendingConn) {
	// CancelSynchronousIo targets a thread, so the worker keeps one thread
	// for its whole life and publishes it to the stopper.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread := s.trackWorkerThread()
	if thread == 0 {
		// Stop could not cancel this worker's blocking pipe I/O: refuse the
		// connection before HELLO_ACK, as at worker capacity.
		pending.Close()
		return
	}
	defer s.untrackWorkerThread(thread)

	sessionID, acceptCfg := s.prepareAcceptConfig()
	s.notifyTestHook(s.handshakeStarted)

	// The SHM objects are created inside the handshake, only for the
	// selected SHM profile.
	var shm *windows.WinShmContext
	prepare := func(profile, requestPayload, responsePayload uint32) bool {
		if profile&winShmProfiles == 0 {
			return true
		}
		shm = s.createShm(sessionID, profile, requestPayload, responsePayload)
		return shm != nil
	}
	session, err := pending.HandshakePrepared(
		sessionID, acceptCfg, s.timings.handshakeMs, s.stopChan(), prepare)
	if err != nil {
		if shm != nil {
			shm.WinShmDestroy()
		}
		return
	}

	if session.SelectedProfile == windows.WinShmProfileHybrid ||
		session.SelectedProfile == windows.WinShmProfileBusywait {
		// SHM profile guarantee: the prepare step created the selected
		// profile's objects before the ACK selected it.
		if shm == nil {
			session.Close()
			return
		}
	} else if shm != nil {
		shm.WinShmDestroy()
		shm = nil
	}

	s.handleSession(session, shm)
}

var (
	serverKernel32          = syscall.NewLazyDLL("kernel32.dll")
	procOpenThread          = serverKernel32.NewProc("OpenThread")
	procCancelSynchronousIo = serverKernel32.NewProc("CancelSynchronousIo")
	procGetCurrentThreadID  = serverKernel32.NewProc("GetCurrentThreadId")
	threadTerminateAccess   = uintptr(0x0001)
)

// openWorkerThread opens a cancellable handle to thread tid, 0 on failure.
// Tests replace it to fail.
var openWorkerThread = func(tid uintptr) syscall.Handle {
	h, _, _ := procOpenThread.Call(threadTerminateAccess, 0, tid)
	return syscall.Handle(h)
}

// trackWorkerThread opens a handle to the calling (locked) OS thread and
// registers it for cancellation. Returns 0 if the handle cannot be opened.
func (s *Server) trackWorkerThread() syscall.Handle {
	tid, _, _ := procGetCurrentThreadID.Call()
	thread := openWorkerThread(tid)
	if thread == 0 {
		return 0
	}
	s.workerThreadsMu.Lock()
	if s.workerThreads == nil {
		s.workerThreads = make(map[syscall.Handle]struct{})
	}
	s.workerThreads[thread] = struct{}{}
	s.workerThreadsMu.Unlock()
	return thread
}

// untrackWorkerThread runs under the stopper's lock, so a cancel never
// reaches a thread that went back to the Go scheduler.
func (s *Server) untrackWorkerThread(thread syscall.Handle) {
	if thread == 0 {
		return
	}
	s.workerThreadsMu.Lock()
	delete(s.workerThreads, thread)
	syscall.CloseHandle(thread)
	s.workerThreadsMu.Unlock()
}

// stopSessionWorkers gives workers a grace period to finish on their own,
// then cancels their blocking pipe I/O until they exit. A single cancel can
// land before a worker enters its next blocking call, so it repeats.
func (s *Server) stopSessionWorkers() {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.After(s.timings.stopGrace):
	}

	ticker := time.NewTicker(serverStopRetry)
	defer ticker.Stop()
	for {
		s.workerThreadsMu.Lock()
		for thread := range s.workerThreads {
			_, _, _ = procCancelSynchronousIo.Call(uintptr(thread))
		}
		s.workerThreadsMu.Unlock()

		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) handleSession(session *windows.Session, shm *windows.WinShmContext) {
	s.handleServerSession(serverSessionOps{
		maxRequestPayloadBytes:  session.MaxRequestPayloadBytes,
		maxResponsePayloadBytes: session.MaxResponsePayloadBytes,
		receive: func(recvBuf []byte) (protocol.Header, []byte, serverReceiveAction) {
			if shm != nil {
				mlen, err := shm.WinShmReceive(recvBuf, s.timings.pollMs)
				if err != nil {
					if err == windows.ErrWinShmTimeout {
						// A crashed client never sets the close flag; its
						// broken pipe is the only sign it left.
						if pipePeerDisconnected(session) {
							return protocol.Header{}, nil, serverReceiveStop
						}
						return protocol.Header{}, nil, serverReceiveContinue
					}
					return protocol.Header{}, nil, serverReceiveStop
				}
				if mlen < protocol.HeaderSize {
					return protocol.Header{}, nil, serverReceiveStop
				}
				hdr, err := protocol.DecodeHeader(recvBuf[:mlen])
				if err != nil {
					return protocol.Header{}, nil, serverReceiveStop
				}
				return hdr, recvBuf[protocol.HeaderSize:mlen], serverReceiveOK
			}

			ready, waitErr := session.WaitReadable(s.timings.pollMs)
			if waitErr != nil {
				return protocol.Header{}, nil, serverReceiveStop
			}
			if !ready {
				return protocol.Header{}, nil, serverReceiveContinue
			}

			hdr, payload, err := session.Receive(recvBuf)
			if err != nil {
				return protocol.Header{}, nil, serverReceiveStop
			}
			return hdr, payload, serverReceiveOK
		},
		send: func(respHdr *protocol.Header, payload []byte, msgBuf *[]byte) error {
			if shm == nil {
				return session.Send(respHdr, payload)
			}
			msg, err := serverEncodeSharedResponse(respHdr, payload, msgBuf)
			if err != nil {
				return err
			}
			return shm.WinShmSend(msg)
		},
		close: func() {
			if shm != nil {
				shm.WinShmDestroy()
			}
			session.Close()
		},
	})
}
