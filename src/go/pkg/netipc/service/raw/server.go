package raw

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const defaultServerWorkerCount = 8

func payloadGrowthCeiling(configuredPayloadBytes uint32) uint32 {
	if configuredPayloadBytes != 0 {
		return configuredPayloadBytes
	}
	return protocol.MaxPayloadCap
}

func (s *Server) initCommon(
	runDir string,
	serviceName string,
	expectedMethodCode uint16,
	handler DispatchHandler,
	workerCount int,
	maxRequestPayloadBytes uint32,
	maxResponsePayloadBytes uint32,
) {
	if workerCount < 1 {
		workerCount = 1
	}
	requestPayloadGrowthCeiling := payloadGrowthCeiling(maxRequestPayloadBytes)
	responsePayloadGrowthCeiling := payloadGrowthCeiling(maxResponsePayloadBytes)
	if maxRequestPayloadBytes == 0 {
		maxRequestPayloadBytes = protocol.MaxPayloadDefault
	}
	if maxResponsePayloadBytes == 0 {
		maxResponsePayloadBytes = protocol.MaxPayloadDefault
	}

	s.runDir = runDir
	s.serviceName = serviceName
	s.expectedMethodCode = expectedMethodCode
	s.handler = handler
	s.workerCount = workerCount
	s.requestPayloadGrowthCeiling = requestPayloadGrowthCeiling
	s.responsePayloadGrowthCeiling = responsePayloadGrowthCeiling
	s.learnedRequestPayloadBytes.Store(maxRequestPayloadBytes)
	s.learnedResponsePayloadBytes.Store(maxResponsePayloadBytes)
}

func (s *Server) retryAcceptAfter(cleanup func()) bool {
	if cleanup != nil {
		cleanup()
	}
	if !s.running.Load() {
		return false
	}
	time.Sleep(10 * time.Millisecond)
	return true
}

func (s *Server) acquireWorkerSlot(sem chan struct{}, cleanup func(), closeSession func()) bool {
	select {
	case sem <- struct{}{}:
		return true
	default:
		if cleanup != nil {
			cleanup()
		}
		closeSession()
		return false
	}
}

func (s *Server) startSessionWorker(sem chan struct{}, run func()) {
	s.wg.Add(1)
	go func() {
		defer func() {
			if recover() != nil {
				// Session handler panicked; keep the accept loop alive.
			}
			<-sem
			s.wg.Done()
		}()
		run()
	}()
}

func (s *Server) dispatchSingle(methodCode uint16, request []byte, responseBuf []byte) (int, error) {
	if methodCode != s.expectedMethodCode || s.handler == nil {
		return 0, errHandlerFailed
	}

	return s.handler(request, responseBuf)
}

func (s *Server) methodSupported(methodCode uint16) bool {
	return s.handler != nil && methodCode == s.expectedMethodCode
}

func serverNotePayloadCapacity(target *atomic.Uint32, payloadLen uint32, ceiling uint32) {
	grown := nextPowerOf2U32(payloadLen)
	if grown > ceiling {
		grown = ceiling
	}
	for {
		current := target.Load()
		if grown <= current {
			return
		}
		if target.CompareAndSwap(current, grown) {
			return
		}
	}
}

type serverReceiveAction uint8

const (
	serverReceiveOK serverReceiveAction = iota
	serverReceiveContinue
	serverReceiveStop
)

type serverSessionOps struct {
	maxRequestPayloadBytes  uint32
	maxResponsePayloadBytes uint32
	receive                 func([]byte) (protocol.Header, []byte, serverReceiveAction)
	send                    func(*protocol.Header, []byte, *[]byte) error
	close                   func()
}

func (s *Server) handleServerSession(ops serverSessionOps) {
	recvBuf := make([]byte, protocol.HeaderSize+int(ops.maxRequestPayloadBytes))
	respBuf := make([]byte, int(ops.maxResponsePayloadBytes))
	itemRespBuf := make([]byte, int(ops.maxResponsePayloadBytes))
	msgBuf := make([]byte, int(ops.maxResponsePayloadBytes)+protocol.HeaderSize)

	defer ops.close()

	for s.running.Load() {
		hdr, payload, action := ops.receive(recvBuf)
		switch action {
		case serverReceiveContinue:
			continue
		case serverReceiveStop:
			return
		}

		if hdr.Kind != protocol.KindRequest {
			return
		}

		respHdr, responseLen, closeAfterSend := s.handleServerRequest(
			hdr, payload, respBuf, itemRespBuf, ops.maxResponsePayloadBytes)
		if err := ops.send(&respHdr, respBuf[:responseLen], &msgBuf); err != nil {
			return
		}
		if closeAfterSend {
			return
		}
	}
}

func (s *Server) handleServerRequest(
	hdr protocol.Header,
	payload []byte,
	respBuf []byte,
	itemRespBuf []byte,
	maxResponsePayloadBytes uint32,
) (protocol.Header, int, bool) {
	if payloadLen, err := checkedLookupU32(len(payload)); err == nil {
		serverNotePayloadCapacity(
			&s.learnedRequestPayloadBytes,
			payloadLen,
			s.requestPayloadGrowthCeiling,
		)
	}

	if !s.methodSupported(hdr.Code) {
		return serverUnsupportedResponseHeader(hdr), 0, false
	}

	responseLen, isBatch, dispatchErr := s.dispatchServerResponse(hdr, payload, respBuf, itemRespBuf)
	respHdr := serverResponseHeader(hdr)

	if dispatchErr == nil {
		if responseLen32, err := checkedLookupU32(responseLen); err == nil {
			serverNotePayloadCapacity(
				&s.learnedResponsePayloadBytes,
				responseLen32,
				s.responsePayloadGrowthCeiling,
			)
		}
		respHdr.TransportStatus = protocol.StatusOK
		if isBatch {
			respHdr.Flags = protocol.FlagBatch
			respHdr.ItemCount = hdr.ItemCount
		} else {
			respHdr.ItemCount = 1
		}
		return respHdr, responseLen, false
	}

	respHdr.ItemCount = 1
	responseLen = 0
	switch {
	case errors.Is(dispatchErr, protocol.ErrOverflow):
		if maxResponsePayloadBytes >= ^uint32(0)/2 {
			serverNotePayloadCapacity(
				&s.learnedResponsePayloadBytes,
				^uint32(0),
				s.responsePayloadGrowthCeiling,
			)
		} else {
			serverNotePayloadCapacity(
				&s.learnedResponsePayloadBytes,
				maxResponsePayloadBytes*2,
				s.responsePayloadGrowthCeiling,
			)
		}
		respHdr.TransportStatus = protocol.StatusLimitExceeded
		return respHdr, responseLen, true
	case errors.Is(dispatchErr, errHandlerFailed):
		respHdr.TransportStatus = protocol.StatusInternalError
	default:
		respHdr.TransportStatus = protocol.StatusBadEnvelope
	}

	return respHdr, responseLen, false
}

func (s *Server) dispatchServerResponse(
	hdr protocol.Header,
	payload []byte,
	respBuf []byte,
	itemRespBuf []byte,
) (int, bool, error) {
	isBatch := (hdr.Flags&protocol.FlagBatch != 0) && hdr.ItemCount >= 1
	if !isBatch {
		responseLen, err := s.dispatchSingle(hdr.Code, payload, respBuf)
		if err != nil {
			return 0, false, err
		}
		if responseLen < 0 || responseLen > len(respBuf) {
			return 0, false, protocol.ErrOverflow
		}
		return responseLen, false, nil
	}

	var bb protocol.BatchBuilder
	bb.Reset(respBuf, hdr.ItemCount)
	for i := uint32(0); i < hdr.ItemCount; i++ {
		itemData, err := protocol.BatchItemGet(payload, hdr.ItemCount, i)
		if err != nil {
			return 0, true, err
		}

		itemResultLen, err := s.dispatchSingle(hdr.Code, itemData, itemRespBuf)
		if err != nil {
			return 0, true, err
		}
		if itemResultLen < 0 || itemResultLen > len(itemRespBuf) {
			return 0, true, protocol.ErrOverflow
		}
		if err := bb.Add(itemRespBuf[:itemResultLen]); err != nil {
			return 0, true, err
		}
	}

	responseLen, _ := bb.Finish()
	return responseLen, true, nil
}

func serverResponseHeader(hdr protocol.Header) protocol.Header {
	return protocol.Header{
		Kind:      protocol.KindResponse,
		Code:      hdr.Code,
		MessageID: hdr.MessageID,
	}
}

func serverUnsupportedResponseHeader(hdr protocol.Header) protocol.Header {
	respHdr := serverResponseHeader(hdr)
	respHdr.TransportStatus = protocol.StatusUnsupported
	respHdr.ItemCount = 1
	return respHdr
}

func serverEncodeSharedResponse(
	respHdr *protocol.Header,
	payload []byte,
	msgBuf *[]byte,
) ([]byte, error) {
	payloadLen, err := checkedLookupU32(len(payload))
	if err != nil {
		return nil, err
	}
	msgLen := protocol.HeaderSize + len(payload)
	if len(*msgBuf) < msgLen {
		*msgBuf = make([]byte, msgLen)
	}
	msg := (*msgBuf)[:msgLen]

	respHdr.Magic = protocol.MagicMsg
	respHdr.Version = protocol.Version
	respHdr.HeaderLen = protocol.HeaderLen
	respHdr.PayloadLen = payloadLen
	respHdr.Encode(msg[:protocol.HeaderSize])
	if len(payload) > 0 {
		copy(msg[protocol.HeaderSize:], payload)
	}
	return msg, nil
}
