//go:build windows

package raw

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// Server is an internal managed server bound to one expected request kind.
type Server struct {
	runDir                      string
	serviceName                 string
	config                      windows.ServerConfig
	expectedMethodCode          uint16
	handler                     DispatchHandler
	running                     atomic.Bool
	learnedRequestPayloadBytes  atomic.Uint32
	learnedResponsePayloadBytes atomic.Uint32
	nextSessionID               atomic.Uint64
	workerCount                 int
	wg                          sync.WaitGroup
	listener                    *windows.Listener // stored so Stop() can close it
}

// NewServer creates a new managed server.
func NewServer(
	runDir, serviceName string,
	config windows.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *Server {
	return NewServerWithWorkers(runDir, serviceName, config, expectedMethodCode, handler, 8)
}

// NewServerWithWorkers creates a server with an explicit worker count limit.
func NewServerWithWorkers(
	runDir, serviceName string,
	config windows.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
	workerCount int,
) *Server {
	if workerCount < 1 {
		workerCount = 1
	}
	learnedRequest := config.MaxRequestPayloadBytes
	if learnedRequest == 0 {
		learnedRequest = protocol.MaxPayloadDefault
	}
	learnedResponse := config.MaxResponsePayloadBytes
	if learnedResponse == 0 {
		learnedResponse = protocol.MaxPayloadDefault
	}
	s := &Server{
		runDir:             runDir,
		serviceName:        serviceName,
		config:             config,
		expectedMethodCode: expectedMethodCode,
		handler:            handler,
		workerCount:        workerCount,
	}
	s.learnedRequestPayloadBytes.Store(learnedRequest)
	s.learnedResponsePayloadBytes.Store(learnedResponse)
	// Session ids are 1-based; prepareAcceptConfig() allocates with Add(1).
	s.nextSessionID.Store(0)
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
	s.listener = listener
	defer func() {
		listener.Close()
		s.listener = nil
	}()

	s.running.Store(true)
	sem := make(chan struct{}, s.workerCount)

	for s.running.Load() {
		sessionID, acceptCfg, preparedShm, ok := s.prepareAcceptConfig()
		if !ok {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		session, err := listener.AcceptWithConfig(sessionID, acceptCfg)
		if err != nil {
			if preparedShm != nil {
				preparedShm.destroyAll()
			}
			if !s.running.Load() {
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		select {
		case sem <- struct{}{}:
		default:
			if preparedShm != nil {
				preparedShm.destroyAll()
			}
			session.Close()
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

		s.wg.Add(1)
		go func(sess *windows.Session, shmCtx *windows.WinShmContext) {
			defer func() {
				if r := recover(); r != nil {
					// Session handler panicked; log but don't crash the server
				}
				<-sem
				s.wg.Done()
			}()
			s.handleSession(sess, shmCtx)
		}(session, shm)
	}

	s.wg.Wait()
	return nil
}

// Stop signals the server to stop and unblocks Accept by closing the listener.
func (s *Server) Stop() {
	s.running.Store(false)
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) handleSession(session *windows.Session, shm *windows.WinShmContext) {
	recvBuf := make([]byte, protocol.HeaderSize+int(session.MaxRequestPayloadBytes))
	respBuf := make([]byte, int(session.MaxResponsePayloadBytes))
	itemRespBuf := make([]byte, int(session.MaxResponsePayloadBytes))
	msgBuf := make([]byte, int(session.MaxResponsePayloadBytes)+protocol.HeaderSize)

	defer func() {
		if shm != nil {
			shm.WinShmDestroy()
		}
		session.Close()
	}()

	for s.running.Load() {
		var hdr protocol.Header
		var payload []byte

		if shm != nil {
			mlen, err := shm.WinShmReceive(recvBuf, serverPollTimeoutMs)
			if err != nil {
				if err == windows.ErrWinShmTimeout {
					continue
				}
				return
			}
			if mlen < protocol.HeaderSize {
				return
			}
			h, err := protocol.DecodeHeader(recvBuf[:mlen])
			if err != nil {
				return
			}
			hdr = h
			payload = recvBuf[protocol.HeaderSize:mlen]
		} else {
			// Named Pipe path
			ready, waitErr := session.WaitReadable(serverPollTimeoutMs)
			if waitErr != nil {
				return
			}
			if !ready {
				continue
			}
			h, p, err := session.Receive(recvBuf)
			if err != nil {
				return
			}
			hdr = h
			payload = p
		}

		// Protocol violation: unexpected message kind terminates session
		if hdr.Kind != protocol.KindRequest {
			return
		}

		if len(payload) <= int(^uint32(0)) {
			serverNotePayloadCapacity(&s.learnedRequestPayloadBytes, uint32(len(payload)))
		}

		if !s.methodSupported(hdr.Code) {
			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            hdr.Code,
				MessageID:       hdr.MessageID,
				TransportStatus: protocol.StatusUnsupported,
				ItemCount:       1,
			}

			if shm != nil {
				if len(msgBuf) < protocol.HeaderSize {
					msgBuf = make([]byte, protocol.HeaderSize)
				}
				msg := msgBuf[:protocol.HeaderSize]
				respHdr.Magic = protocol.MagicMsg
				respHdr.Version = protocol.Version
				respHdr.HeaderLen = protocol.HeaderLen
				respHdr.PayloadLen = 0
				respHdr.Encode(msg[:protocol.HeaderSize])
				if err := shm.WinShmSend(msg); err != nil {
					return
				}
			} else if err := session.Send(&respHdr, nil); err != nil {
				return
			}
			continue
		}

		// Dispatch: single-item or batch
		responseLen := 0
		isBatch := (hdr.Flags&protocol.FlagBatch != 0) && hdr.ItemCount >= 1
		var dispatchErr error

		if !isBatch {
			var derr error
			responseLen, derr = s.dispatchSingle(hdr.Code, payload, respBuf)
			if derr != nil {
				dispatchErr = derr
				responseLen = 0
			} else if responseLen < 0 || responseLen > len(respBuf) {
				dispatchErr = protocol.ErrOverflow
				responseLen = 0
			}
		} else {
			var bb protocol.BatchBuilder
			bb.Reset(respBuf, hdr.ItemCount)

			for i := uint32(0); i < hdr.ItemCount && dispatchErr == nil; i++ {
				itemData, gerr := protocol.BatchItemGet(payload, hdr.ItemCount, i)
				if gerr != nil {
					dispatchErr = gerr
					break
				}

				itemResultLen, derr := s.dispatchSingle(hdr.Code, itemData, itemRespBuf)
				if derr != nil {
					dispatchErr = derr
					break
				}
				if itemResultLen < 0 || itemResultLen > len(itemRespBuf) {
					dispatchErr = protocol.ErrOverflow
					break
				}

				if aerr := bb.Add(itemRespBuf[:itemResultLen]); aerr != nil {
					dispatchErr = aerr
					break
				}
			}

			if dispatchErr == nil {
				responseLen, _ = bb.Finish()
			}
		}

		// Build response header
		respHdr := protocol.Header{
			Kind:      protocol.KindResponse,
			Code:      hdr.Code,
			MessageID: hdr.MessageID,
		}

		if dispatchErr == nil {
			if responseLen <= int(^uint32(0)) {
				serverNotePayloadCapacity(&s.learnedResponsePayloadBytes, uint32(responseLen))
			}
			respHdr.TransportStatus = protocol.StatusOK
			if isBatch {
				respHdr.Flags = protocol.FlagBatch
				respHdr.ItemCount = hdr.ItemCount
			} else {
				respHdr.ItemCount = 1
			}
		} else if errors.Is(dispatchErr, protocol.ErrOverflow) {
			current := session.MaxResponsePayloadBytes
			if current >= ^uint32(0)/2 {
				serverNotePayloadCapacity(&s.learnedResponsePayloadBytes, ^uint32(0))
			} else {
				serverNotePayloadCapacity(&s.learnedResponsePayloadBytes, current*2)
			}
			respHdr.TransportStatus = protocol.StatusLimitExceeded
			respHdr.ItemCount = 1
			responseLen = 0
		} else if errors.Is(dispatchErr, errHandlerFailed) {
			respHdr.TransportStatus = protocol.StatusInternalError
			respHdr.ItemCount = 1
			responseLen = 0
		} else {
			respHdr.TransportStatus = protocol.StatusBadEnvelope
			respHdr.ItemCount = 1
			responseLen = 0
		}

		if shm != nil {
			msgLen := protocol.HeaderSize + responseLen
			if len(msgBuf) < msgLen {
				msgBuf = make([]byte, msgLen)
			}
			msg := msgBuf[:msgLen]

			respHdr.Magic = protocol.MagicMsg
			respHdr.Version = protocol.Version
			respHdr.HeaderLen = protocol.HeaderLen
			respHdr.PayloadLen = uint32(responseLen)

			respHdr.Encode(msg[:protocol.HeaderSize])
			if responseLen > 0 {
				copy(msg[protocol.HeaderSize:], respBuf[:responseLen])
			}

			if err := shm.WinShmSend(msg); err != nil {
				return
			}
			if respHdr.TransportStatus == protocol.StatusLimitExceeded {
				return
			}
		} else {
			if err := session.Send(&respHdr, respBuf[:responseLen]); err != nil {
				return
			}
			if respHdr.TransportStatus == protocol.StatusLimitExceeded {
				return
			}
		}
	}
}
