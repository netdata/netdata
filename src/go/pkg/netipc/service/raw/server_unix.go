//go:build unix

package raw

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// Server is an internal managed server bound to one expected request kind.
// Supports multiple concurrent client sessions up to workerCount.
type Server struct {
	runDir                      string
	serviceName                 string
	config                      posix.ServerConfig
	expectedMethodCode          uint16
	handler                     DispatchHandler
	running                     atomic.Bool
	learnedRequestPayloadBytes  atomic.Uint32
	learnedResponsePayloadBytes atomic.Uint32
	nextSessionID               atomic.Uint64
	workerCount                 int
	wg                          sync.WaitGroup
}

// NewServer creates a new managed server. workerCount limits the
// maximum number of concurrent client sessions (default 1 if <= 0).
func NewServer(
	runDir, serviceName string,
	config posix.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *Server {
	return NewServerWithWorkers(runDir, serviceName, config, expectedMethodCode, handler, 8)
}

// NewServerWithWorkers creates a server with an explicit worker count limit.
func NewServerWithWorkers(
	runDir, serviceName string,
	config posix.ServerConfig,
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
	defer listener.Close()

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
			time.Sleep(10 * time.Millisecond)
			continue
		}

		session, err := listener.AcceptWithConfig(sessionID, acceptCfg)
		if err != nil {
			if precreatedShm != nil {
				precreatedShm.ShmDestroy()
			}
			if !s.running.Load() {
				break
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Try to acquire a worker slot (non-blocking check)
		select {
		case sem <- struct{}{}:
			// Got a slot
		default:
			// At capacity: reject client
			if precreatedShm != nil {
				precreatedShm.ShmDestroy()
			}
			session.Close()
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

		// Handle this session in a goroutine
		s.wg.Add(1)
		go func(sess *posix.Session, shmCtx *posix.ShmContext) {
			defer func() {
				if r := recover(); r != nil {
					// Session handler panicked; log but don't crash the server
				}
				<-sem // release worker slot
				s.wg.Done()
			}()
			s.handleSession(sess, shmCtx)
		}(session, shm)
	}

	// Wait for all active session goroutines to finish
	s.wg.Wait()

	return nil
}

// Stop signals the server to stop.
func (s *Server) Stop() {
	s.running.Store(false)
}

func (s *Server) handleSession(session *posix.Session, shm *posix.ShmContext) {
	recvBuf := make([]byte, protocol.HeaderSize+int(session.MaxRequestPayloadBytes))
	respBuf := make([]byte, int(session.MaxResponsePayloadBytes))
	itemRespBuf := make([]byte, int(session.MaxResponsePayloadBytes))
	msgBuf := make([]byte, int(session.MaxResponsePayloadBytes)+protocol.HeaderSize)

	defer func() {
		if shm != nil {
			shm.ShmDestroy()
		}
		session.Close()
	}()

	for s.running.Load() {
		var hdr protocol.Header
		var payload []byte

		if shm != nil {
			mlen, err := shm.ShmReceive(recvBuf, serverPollTimeoutMs)
			if err != nil {
				if err == posix.ErrShmTimeout {
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
			// Poll the session fd before blocking on receive
			ready := pollFd(session.Fd(), serverPollTimeoutMs)
			if ready < 0 {
				return
			}
			if ready == 0 {
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

		if payloadLen, err := checkedLookupU32(len(payload)); err == nil {
			serverNotePayloadCapacity(&s.learnedRequestPayloadBytes, payloadLen)
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
				if err := shm.ShmSend(msg); err != nil {
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

		// Send response via the active transport
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

			if err := shm.ShmSend(msg); err != nil {
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
