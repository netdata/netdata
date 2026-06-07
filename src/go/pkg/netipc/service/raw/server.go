package raw

import "sync/atomic"

func (s *Server) dispatchSingle(methodCode uint16, request []byte, responseBuf []byte) (int, error) {
	if methodCode != s.expectedMethodCode || s.handler == nil {
		return 0, errHandlerFailed
	}

	return s.handler(request, responseBuf)
}

func (s *Server) methodSupported(methodCode uint16) bool {
	return s.handler != nil && methodCode == s.expectedMethodCode
}

func serverNotePayloadCapacity(target *atomic.Uint32, payloadLen uint32) {
	grown := nextPowerOf2U32(payloadLen)
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
