//go:build unix

package raw

import (
	"sync"
	"sync/atomic"

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
	listenerMu                   sync.Mutex
	listener                     *posix.Listener
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

func (s *Server) prepareAcceptConfig() (uint64, posix.ServerConfig, *posix.ShmContext, bool) {
	sessionID := s.nextSessionID.Add(1)
	cfg := s.config
	cfg.MaxRequestPayloadBytes = s.learnedRequestPayloadBytes.Load()
	cfg.MaxResponsePayloadBytes = s.learnedResponsePayloadBytes.Load()

	if cfg.SupportedProfiles&posixShmProfiles == 0 {
		return sessionID, cfg, nil, true
	}

	shm, err := posix.ShmServerCreate(
		s.runDir, s.serviceName, sessionID,
		cfg.MaxRequestPayloadBytes+uint32(protocol.HeaderSize),
		cfg.MaxResponsePayloadBytes+uint32(protocol.HeaderSize),
	)
	if err == nil {
		return sessionID, cfg, shm, true
	}

	cfg.SupportedProfiles &^= posixShmProfiles
	cfg.PreferredProfiles &^= posixShmProfiles
	if cfg.SupportedProfiles == 0 {
		return sessionID, cfg, nil, false
	}

	return sessionID, cfg, nil, true
}

// Run starts the acceptor loop. Blocking. Accepts clients, spawns a
// goroutine per session (up to workerCount concurrently).
// Returns when Stop() is called or on fatal error.
func (s *Server) Run() error {
	posix.ShmCleanupStale(s.runDir, s.serviceName)

	listener, err := posix.Listen(s.runDir, s.serviceName, s.config)
	if err != nil {
		return err
	}
	s.setListener(listener)
	defer s.closeListener(listener)

	s.running.Store(true)

	/* Semaphore channel limits concurrent sessions */
	sem := make(chan struct{}, s.workerCount)

	for s.running.Load() {
		// Poll the listener fd before blocking on accept
		ready := pollFd(listener.Fd(), serverPollTimeoutMs)
		if ready < 0 {
			break
		}
		if ready == 0 {
			continue
		}

		sessionID, acceptCfg, precreatedShm, ok := s.prepareAcceptConfig()
		if !ok {
			s.retryAcceptAfter(nil)
			continue
		}

		session, err := listener.AcceptWithConfig(sessionID, acceptCfg)
		if err != nil {
			if !s.retryAcceptAfter(func() {
				if precreatedShm != nil {
					precreatedShm.ShmDestroy()
				}
			}) {
				break
			}
			continue
		}

		if !s.acquireWorkerSlot(sem, func() {
			if precreatedShm != nil {
				precreatedShm.ShmDestroy()
			}
		}, session.Close) {
			continue
		}

		var shm *posix.ShmContext
		if session.SelectedProfile == protocol.ProfileSHMHybrid ||
			session.SelectedProfile == protocol.ProfileSHMFutex {
			if precreatedShm == nil {
				session.Close()
				<-sem
				continue
			}
			shm = precreatedShm
		} else if precreatedShm != nil {
			precreatedShm.ShmDestroy()
		}

		s.startSessionWorker(sem, func() {
			s.handleSession(session, shm)
		})
	}

	// Wait for all active session goroutines to finish
	s.wg.Wait()

	return nil
}

// Stop signals the server to stop and unblocks Accept by closing the listener.
func (s *Server) Stop() {
	s.running.Store(false)
	s.closeListener(nil)
}

func (s *Server) setListener(listener *posix.Listener) {
	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()
}

func (s *Server) closeListener(listener *posix.Listener) {
	s.listenerMu.Lock()
	target := listener
	if target == nil {
		target = s.listener
	}
	if target != nil && s.listener == target {
		s.listener = nil
	}
	s.listenerMu.Unlock()

	if target != nil {
		target.Close()
	}
}

func (s *Server) handleSession(session *posix.Session, shm *posix.ShmContext) {
	s.handleServerSession(serverSessionOps{
		maxRequestPayloadBytes:  session.MaxRequestPayloadBytes,
		maxResponsePayloadBytes: session.MaxResponsePayloadBytes,
		receive: func(recvBuf []byte) (protocol.Header, []byte, serverReceiveAction) {
			if shm != nil {
				mlen, err := shm.ShmReceive(recvBuf, serverPollTimeoutMs)
				if err != nil {
					if err == posix.ErrShmTimeout {
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

			ready := pollFd(session.Fd(), serverPollTimeoutMs)
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
			session.Close()
		},
	})
}
