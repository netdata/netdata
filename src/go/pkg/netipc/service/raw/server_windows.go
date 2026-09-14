//go:build windows

package raw

import (
	"sync"
	"sync/atomic"

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
	listener                     *windows.Listener // stored so Stop() can close it
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

type preparedWinShm struct {
	hybrid   *windows.WinShmContext
	busywait *windows.WinShmContext
}

func (p *preparedWinShm) take(profile uint32) *windows.WinShmContext {
	if p == nil {
		return nil
	}
	switch profile {
	case windows.WinShmProfileHybrid:
		ctx := p.hybrid
		p.hybrid = nil
		return ctx
	case windows.WinShmProfileBusywait:
		ctx := p.busywait
		p.busywait = nil
		return ctx
	default:
		return nil
	}
}

func (p *preparedWinShm) destroyAll() {
	if p == nil {
		return
	}
	if p.hybrid != nil {
		p.hybrid.WinShmDestroy()
		p.hybrid = nil
	}
	if p.busywait != nil {
		p.busywait.WinShmDestroy()
		p.busywait = nil
	}
}

const winShmProfiles = windows.WinShmProfileHybrid | windows.WinShmProfileBusywait

func (s *Server) prepareAcceptConfig() (uint64, windows.ServerConfig, *preparedWinShm, bool) {
	sessionID := s.nextSessionID.Add(1)
	cfg := s.config
	cfg.MaxRequestPayloadBytes = s.learnedRequestPayloadBytes.Load()
	cfg.MaxResponsePayloadBytes = s.learnedResponsePayloadBytes.Load()

	if cfg.SupportedProfiles&winShmProfiles == 0 {
		return sessionID, cfg, nil, true
	}

	prepared := &preparedWinShm{}
	for _, profile := range []uint32{windows.WinShmProfileHybrid, windows.WinShmProfileBusywait} {
		if cfg.SupportedProfiles&profile == 0 {
			continue
		}
		shm, err := windows.WinShmServerCreate(
			s.runDir, s.serviceName,
			cfg.AuthToken,
			sessionID,
			profile,
			cfg.MaxRequestPayloadBytes+uint32(protocol.HeaderSize),
			cfg.MaxResponsePayloadBytes+uint32(protocol.HeaderSize),
		)
		if err != nil {
			cfg.SupportedProfiles &^= profile
			cfg.PreferredProfiles &^= profile
			continue
		}
		if profile == windows.WinShmProfileHybrid {
			prepared.hybrid = shm
		} else {
			prepared.busywait = shm
		}
	}

	if cfg.SupportedProfiles == 0 {
		prepared.destroyAll()
		return sessionID, cfg, nil, false
	}

	if prepared.hybrid == nil && prepared.busywait == nil {
		return sessionID, cfg, nil, true
	}

	return sessionID, cfg, prepared, true
}

// Run starts the acceptor loop. Blocking.
func (s *Server) Run() error {
	listener, err := windows.Listen(s.runDir, s.serviceName, s.config)
	if err != nil {
		return err
	}
	s.setListener(listener)
	defer s.closeListener(listener)

	s.running.Store(true)
	sem := make(chan struct{}, s.workerCount)

	for s.running.Load() {
		sessionID, acceptCfg, preparedShm, ok := s.prepareAcceptConfig()
		if !ok {
			s.retryAcceptAfter(nil)
			continue
		}

		session, err := listener.AcceptWithConfig(sessionID, acceptCfg)
		if err != nil {
			if !s.retryAcceptAfter(func() {
				if preparedShm != nil {
					preparedShm.destroyAll()
				}
			}) {
				break
			}
			continue
		}

		if !s.acquireWorkerSlot(sem, func() {
			if preparedShm != nil {
				preparedShm.destroyAll()
			}
		}, session.Close) {
			continue
		}

		var shm *windows.WinShmContext
		if session.SelectedProfile == windows.WinShmProfileHybrid ||
			session.SelectedProfile == windows.WinShmProfileBusywait {
			shm = preparedShm.take(session.SelectedProfile)
			if shm == nil {
				if preparedShm != nil {
					preparedShm.destroyAll()
				}
				session.Close()
				<-sem
				continue
			}
		}
		if preparedShm != nil {
			preparedShm.destroyAll()
		}

		s.startSessionWorker(sem, func() {
			s.handleSession(session, shm)
		})
	}

	s.wg.Wait()
	return nil
}

// Stop signals the server to stop and unblocks Accept by closing the listener.
func (s *Server) Stop() {
	s.running.Store(false)
	s.closeListener(nil)
}

func (s *Server) setListener(listener *windows.Listener) {
	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()
}

func (s *Server) closeListener(listener *windows.Listener) {
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

func (s *Server) handleSession(session *windows.Session, shm *windows.WinShmContext) {
	s.handleServerSession(serverSessionOps{
		maxRequestPayloadBytes:  session.MaxRequestPayloadBytes,
		maxResponsePayloadBytes: session.MaxResponsePayloadBytes,
		receive: func(recvBuf []byte) (protocol.Header, []byte, serverReceiveAction) {
			if shm != nil {
				mlen, err := shm.WinShmReceive(recvBuf, serverPollTimeoutMs)
				if err != nil {
					if err == windows.ErrWinShmTimeout {
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

			ready, waitErr := session.WaitReadable(serverPollTimeoutMs)
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
