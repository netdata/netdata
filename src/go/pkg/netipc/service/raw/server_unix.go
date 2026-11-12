//go:build unix

package raw

import (
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// Server is an internal managed server bound to one expected request kind.
// Supports multiple concurrent client sessions up to workerCount.
type Server struct {
	runDir                       string
	serviceName                  string
	config                       posix.ServerConfig
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
	stopRequested                atomic.Bool
	stopInit                     sync.Once
	stopClose                    sync.Once
	stopCh                       chan struct{}
	timings                      serverTimings
	listening                    chan struct{} // closed once the listener accepts
	listeningOnce                sync.Once
	sessionExited                chan struct{} // test hook, nil in production: signalled after a worker frees its slot
	handshakeStarted             chan struct{} // test hook, nil in production: signalled before a worker waits for HELLO
	shmCreated                   chan struct{} // test hook, nil in production: signalled once a session's SHM region exists
	failShmCreate                atomic.Bool   // test hook: makes SHM region creation fail
	sessionFdsMu                 sync.Mutex
	sessionFds                   map[int]struct{}
}

// NewServer creates a new managed server. workerCount limits the
// maximum number of concurrent client sessions (default 1 if <= 0).
func NewServer(
	runDir, serviceName string,
	config posix.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *Server {
	return NewServerWithWorkers(runDir, serviceName, config, expectedMethodCode, handler, defaultServerWorkerCount)
}

// NewServerWithWorkers creates a server with an explicit worker count limit.
func NewServerWithWorkers(
	runDir, serviceName string,
	config posix.ServerConfig,
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

const posixShmProfiles = protocol.ProfileSHMHybrid | protocol.ProfileSHMFutex

func (s *Server) prepareAcceptConfig() (uint64, posix.ServerConfig) {
	sessionID := s.nextSessionID.Add(1)
	cfg := s.config
	cfg.MaxRequestPayloadBytes = s.learnedRequestPayloadBytes.Load()
	cfg.MaxResponsePayloadBytes = s.learnedResponsePayloadBytes.Load()
	return sessionID, cfg
}

// createShm creates a session's SHM region for the agreed payload limits.
func (s *Server) createShm(sessionID uint64, requestPayload, responsePayload uint32) *posix.ShmContext {
	if s.failShmCreate.Load() {
		return nil
	}
	reqCap, reqOK := headerPayloadLen(requestPayload)
	respCap, respOK := headerPayloadLen(responsePayload)
	if !reqOK || !respOK {
		return nil
	}
	shm, err := posix.ShmServerCreate(s.runDir, s.serviceName, sessionID, reqCap, respCap)
	if err != nil {
		return nil
	}
	s.notifyTestHook(s.shmCreated)
	return shm
}

// Run starts the acceptor loop. Blocking. Accepts clients and runs each
// connection, handshake included, in its own goroutine (up to workerCount
// concurrently). Returns when Stop() is called or on fatal error. The server
// is single-use: a Stop before Run makes Run return at once.
func (s *Server) Run() error {
	if s.stopRequested.Load() {
		return nil
	}

	posix.ShmCleanupStale(s.runDir, s.serviceName)

	listener, err := posix.Listen(s.runDir, s.serviceName, s.config)
	if err != nil {
		return err
	}
	defer listener.Close()

	s.running.Store(true)
	// Stop may have run between the check above and the store.
	if s.stopRequested.Load() {
		s.running.Store(false)
	}
	s.markListening()

	/* Semaphore channel limits concurrent sessions */
	sem := make(chan struct{}, s.workerCount)

	for s.running.Load() {
		// Poll the listener fd before blocking on accept
		ready := pollFd(listener.Fd(), int(s.timings.pollMs))
		if ready < 0 {
			break
		}
		if ready == 0 {
			continue
		}

		pending, err := listener.AcceptPending()
		if err != nil {
			if !s.retryAcceptAfter(nil) {
				break
			}
			continue
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
	// Close the socket now; the deferred Close releases the endpoint lock
	// after the workers have exited.
	listener.StopListening()
	s.stopSessionWorkers()

	return nil
}

// Stop signals the server to stop. Run owns the listener and closes it on
// exit, so Stop never touches a descriptor Run may still be polling.
func (s *Server) Stop() {
	s.markStopped()
}

func (s *Server) serveConnection(pending *posix.PendingConn) {
	sessionID, acceptCfg := s.prepareAcceptConfig()
	s.notifyTestHook(s.handshakeStarted)

	// The region is created inside the handshake, only once SHM is selected.
	var shm *posix.ShmContext
	prepare := func(profile, requestPayload, responsePayload uint32) bool {
		if profile&posixShmProfiles == 0 {
			return true
		}
		shm = s.createShm(sessionID, requestPayload, responsePayload)
		return shm != nil
	}
	session, err := pending.HandshakePrepared(
		sessionID, acceptCfg, s.timings.handshakeMs, s.stopChan(), prepare)
	if err != nil {
		if shm != nil {
			shm.ShmDestroy()
		}
		return
	}

	if session.SelectedProfile == protocol.ProfileSHMHybrid ||
		session.SelectedProfile == protocol.ProfileSHMFutex {
		// SHM profile guarantee: the prepare step created the region before
		// the ACK selected SHM.
		if shm == nil {
			session.Close()
			return
		}
	} else if shm != nil {
		shm.ShmDestroy()
		shm = nil
	}

	s.trackSessionFd(session.Fd())
	s.handleSession(session, shm)
}

func (s *Server) trackSessionFd(fd int) {
	s.sessionFdsMu.Lock()
	if s.sessionFds == nil {
		s.sessionFds = make(map[int]struct{})
	}
	s.sessionFds[fd] = struct{}{}
	s.sessionFdsMu.Unlock()
}

// closeTrackedSession closes a session under the same lock the stopper uses
// to shut sessions down, so a shutdown never reaches a reused fd number.
func (s *Server) closeTrackedSession(session *posix.Session) {
	s.sessionFdsMu.Lock()
	delete(s.sessionFds, session.Fd())
	session.Close()
	s.sessionFdsMu.Unlock()
}

// stopSessionWorkers gives workers a grace period to finish on their own,
// then shuts their sockets down so a worker blocked writing to a client that
// stopped reading wakes up.
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
		s.sessionFdsMu.Lock()
		for fd := range s.sessionFds {
			_ = syscall.Shutdown(fd, syscall.SHUT_RDWR)
		}
		s.sessionFdsMu.Unlock()

		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) handleSession(session *posix.Session, shm *posix.ShmContext) {
	s.handleServerSession(serverSessionOps{
		maxRequestPayloadBytes:  session.MaxRequestPayloadBytes,
		maxResponsePayloadBytes: session.MaxResponsePayloadBytes,
		receive: func(recvBuf []byte) (protocol.Header, []byte, serverReceiveAction) {
			if shm != nil {
				mlen, err := shm.ShmReceive(recvBuf, s.timings.pollMs)
				if err != nil {
					if err == posix.ErrShmTimeout {
						// A client that left (cleanly or by crashing)
						// never sends again; its socket is the only sign.
						if peerDisconnected(session.Fd()) {
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

			ready := pollFd(session.Fd(), int(s.timings.pollMs))
			if ready < 0 {
				return protocol.Header{}, nil, serverReceiveStop
			}
			if ready == 0 {
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
			return shm.ShmSend(msg)
		},
		close: func() {
			if shm != nil {
				shm.ShmDestroy()
			}
			s.closeTrackedSession(session)
		},
	})
}
