package raw

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const clientShmAttachRetryInterval = 5 * time.Millisecond
const clientShmAttachRetryTimeout = 5 * time.Second

func nextPowerOf2U32(n uint32) uint32 {
	if n < 16 {
		return 16
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

func (c *Client) validateMethod(methodCode uint16) error {
	if c.expectedMethodCode != methodCode {
		return protocol.ErrBadLayout
	}
	return nil
}

// Refresh attempts connect if DISCONNECTED/NOT_FOUND, reconnect if BROKEN.
// Returns true if the state changed.
func (c *Client) Refresh() bool {
	oldState := c.state

	switch c.state {
	case StateDisconnected, StateNotFound:
		c.state = StateConnecting
		c.state = c.tryConnect()
		if c.state == StateReady {
			c.connectCount++
		}

	case StateBroken:
		c.disconnect()
		c.state = StateConnecting
		c.state = c.tryConnect()
		if c.state == StateReady {
			c.reconnectCount++
		}

	case StateReady, StateConnecting, StateAuthFailed, StateIncompatible:
		// No action needed
	}

	return c.state != oldState
}

// Ready returns true only if the client is in the READY state.
// Cheap cached boolean, no I/O.
func (c *Client) Ready() bool {
	return c.state == StateReady
}

// Status returns a diagnostic counters snapshot.
func (c *Client) Status() ClientStatus {
	return ClientStatus{
		State:          c.state,
		ConnectCount:   c.connectCount,
		ReconnectCount: c.reconnectCount,
		CallCount:      c.callCount,
		ErrorCount:     c.errorCount,
	}
}

func (c *Client) sessionMaxRequestPayloadBytes() uint32 {
	if c.session != nil {
		return c.session.MaxRequestPayloadBytes
	}
	return c.config.MaxRequestPayloadBytes
}

func (c *Client) sessionMaxResponsePayloadBytes() uint32 {
	if c.session != nil {
		return c.session.MaxResponsePayloadBytes
	}
	return c.config.MaxResponsePayloadBytes
}

func (c *Client) noteRequestCapacity(payloadLen uint32) {
	grown := min(nextPowerOf2U32(payloadLen), protocol.MaxPayloadCap)
	if grown > c.config.MaxRequestPayloadBytes {
		c.config.MaxRequestPayloadBytes = grown
	}
}

func (c *Client) noteResponseCapacity(payloadLen uint32) {
	grown := min(nextPowerOf2U32(payloadLen), protocol.MaxPayloadCap)
	if grown > c.config.MaxResponsePayloadBytes {
		c.config.MaxResponsePayloadBytes = grown
	}
}

// callWithRetry manages reconnect-driven recovery for a typed call.
// Ordinary failures retry once. Overflow-driven resize recovery may
// reconnect more than once while negotiated capacities grow.
func (c *Client) callWithRetry(attempt func() error) error {
	// Fail fast if not READY
	if c.state != StateReady {
		c.errorCount++
		return protocol.ErrBadLayout
	}

	// Cap overflow-driven retries: payloads grow by powers of 2, so 8
	// retries allows ~256x growth from the initial negotiated size.
	overflowRetries := 0
	for {
		prevReq := c.sessionMaxRequestPayloadBytes()
		prevResp := c.sessionMaxResponsePayloadBytes()
		prevCfgReq := c.config.MaxRequestPayloadBytes
		prevCfgResp := c.config.MaxResponsePayloadBytes

		firstErr := attempt()
		if firstErr == nil {
			c.callCount++
			return nil
		}

		if !errors.Is(firstErr, protocol.ErrOverflow) {
			c.disconnect()
			c.state = StateBroken

			c.state = c.tryConnect()
			if c.state != StateReady {
				c.errorCount++
				return firstErr
			}
			c.reconnectCount++

			retryErr := attempt()
			if retryErr == nil {
				c.callCount++
				return nil
			}

			c.disconnect()
			c.state = StateBroken
			c.errorCount++
			return retryErr
		}

		c.disconnect()
		c.state = StateBroken

		c.state = c.tryConnect()
		if c.state != StateReady {
			c.errorCount++
			return firstErr
		}
		c.reconnectCount++

		if c.sessionMaxRequestPayloadBytes() <= prevReq &&
			c.sessionMaxResponsePayloadBytes() <= prevResp &&
			c.config.MaxRequestPayloadBytes <= prevCfgReq &&
			c.config.MaxResponsePayloadBytes <= prevCfgResp {
			c.disconnect()
			c.state = StateBroken
			c.errorCount++
			return firstErr
		}

		overflowRetries++
		if overflowRetries >= 8 {
			c.disconnect()
			c.state = StateBroken
			c.errorCount++
			return firstErr
		}
	}
}

// doRawCall sends a request and receives/validates the response envelope.
// Returns the validated response header and a borrowed payload view.
func (c *Client) doRawCall(methodCode uint16, reqPayload []byte) (protocol.Header, []byte, error) {
	hdr := protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            methodCode,
		Flags:           0,
		ItemCount:       1,
		MessageID:       uint64(c.callCount) + 1,
		TransportStatus: protocol.StatusOK,
	}

	if err := c.transportSend(&hdr, reqPayload); err != nil {
		return protocol.Header{}, nil, err
	}

	respHdr, payload, err := c.transportReceive()
	if err != nil {
		return protocol.Header{}, nil, err
	}

	if respHdr.Kind != protocol.KindResponse {
		return protocol.Header{}, nil, protocol.ErrBadKind
	}
	if respHdr.Code != methodCode {
		return protocol.Header{}, nil, protocol.ErrBadLayout
	}
	if respHdr.MessageID != hdr.MessageID {
		return protocol.Header{}, nil, protocol.ErrBadLayout
	}
	switch respHdr.TransportStatus {
	case protocol.StatusOK:
	case protocol.StatusLimitExceeded:
		if current := c.sessionMaxResponsePayloadBytes(); current > 0 {
			if current >= ^uint32(0)/2 {
				c.noteResponseCapacity(^uint32(0))
			} else {
				c.noteResponseCapacity(current * 2)
			}
		}
		return protocol.Header{}, nil, protocol.ErrOverflow
	default:
		return protocol.Header{}, nil, protocol.ErrBadLayout
	}

	return respHdr, payload, nil
}

// Close tears down the connection and releases resources.
func (c *Client) Close() {
	c.disconnect()
	c.state = StateDisconnected
}
