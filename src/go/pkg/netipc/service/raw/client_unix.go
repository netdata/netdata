//go:build unix

package raw

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// Client is an internal L2 client context bound to one service kind.
// It manages connection lifecycle and provides typed blocking calls with
// at-least-once retry semantics.
type Client struct {
	state              ClientState
	runDir             string
	serviceName        string
	expectedMethodCode uint16
	config             posix.ClientConfig

	// Connection (managed internally)
	session *posix.Session
	shm     *posix.ShmContext

	// Reusable scratch buffers owned by the client for hot request paths.
	requestBuf   []byte
	sendBuf      []byte
	transportBuf []byte

	// Stats
	connectCount   uint32
	reconnectCount uint32
	callCount      uint32
	errorCount     uint32
}

func newClient(runDir, serviceName string, config posix.ClientConfig, expectedMethodCode uint16) *Client {
	return &Client{
		state:              StateDisconnected,
		runDir:             runDir,
		serviceName:        serviceName,
		expectedMethodCode: expectedMethodCode,
		config:             config,
	}
}

func (c *Client) disconnect() {
	if c.shm != nil {
		c.shm.ShmClose()
		c.shm = nil
	}
	if c.session != nil {
		c.session.Close()
		c.session = nil
	}
}

func (c *Client) tryConnect() ClientState {
	session, err := posix.Connect(c.runDir, c.serviceName, &c.config)
	if err != nil {
		switch {
		case isConnectError(err):
			return StateNotFound
		case isAuthError(err):
			return StateAuthFailed
		case isIncompatibleError(err):
			return StateIncompatible
		default:
			return StateDisconnected
		}
	}

	// SHM upgrade if negotiated
	if session.SelectedProfile == protocol.ProfileSHMHybrid ||
		session.SelectedProfile == protocol.ProfileSHMFutex {
		// Retry attach: the server prepared SHM before handshake, but the
		// client may still race slightly with the peer exposing the region.
		deadline := time.Now().Add(clientShmAttachRetryTimeout)
		for {
			shm, serr := posix.ShmClientAttach(c.runDir, c.serviceName, session.SessionID)
			if serr == nil {
				c.shm = shm
				break
			}
			if !time.Now().Before(deadline) {
				break
			}
			time.Sleep(clientShmAttachRetryInterval)
		}
		if c.shm == nil {
			// SHM attach failed after negotiation. Close that session,
			// blacklist SHM for this client context, and retry baseline.
			session.Close()
			c.config.SupportedProfiles &^= posixShmProfiles
			c.config.PreferredProfiles &^= posixShmProfiles
			if c.config.SupportedProfiles == 0 {
				return StateDisconnected
			}
			return c.tryConnect()
		}
	}

	c.session = session
	return StateReady
}

func (c *Client) transportSend(hdr *protocol.Header, payload []byte) error {
	payloadLen, err := checkedLookupU32(len(payload))
	if err != nil {
		c.noteRequestCapacity(^uint32(0))
		return protocol.ErrOverflow
	}

	if c.shm != nil {
		if payloadLen > c.sessionMaxRequestPayloadBytes() {
			c.noteRequestCapacity(payloadLen)
			return protocol.ErrOverflow
		}

		msgLen, err := checkedLookupAdd(protocol.HeaderSize, len(payload))
		if err != nil {
			return protocol.ErrOverflow
		}
		msg := ensureClientScratch(&c.sendBuf, msgLen)

		hdr.Magic = protocol.MagicMsg
		hdr.Version = protocol.Version
		hdr.HeaderLen = protocol.HeaderLen
		hdr.PayloadLen = payloadLen

		hdr.Encode(msg[:protocol.HeaderSize])
		if len(payload) > 0 {
			copy(msg[protocol.HeaderSize:], payload)
		}

		if err := c.shm.ShmSend(msg[:msgLen]); err != nil {
			if errors.Is(err, posix.ErrShmMsgTooLarge) {
				c.noteRequestCapacity(payloadLen)
				return protocol.ErrOverflow
			}
			return protocol.ErrTruncated
		}
		return nil
	}

	// UDS path
	if c.session == nil {
		return protocol.ErrTruncated
	}
	if err := c.session.Send(hdr, payload); err != nil {
		if errors.Is(err, posix.ErrLimitExceeded) {
			c.noteRequestCapacity(payloadLen)
			return protocol.ErrOverflow
		}
		return protocol.ErrTruncated
	}
	return nil
}

func (c *Client) transportReceive() (protocol.Header, []byte, error) {
	scratch := ensureClientScratch(&c.transportBuf, c.maxReceiveMessageBytes())

	if c.shm != nil {
		mlen, err := c.shm.ShmReceive(scratch, 30000)
		if err != nil {
			return protocol.Header{}, nil, protocol.ErrTruncated
		}
		if mlen < protocol.HeaderSize {
			return protocol.Header{}, nil, protocol.ErrTruncated
		}

		hdr, err := protocol.DecodeHeader(scratch[:mlen])
		if err != nil {
			return protocol.Header{}, nil, err
		}
		return hdr, scratch[protocol.HeaderSize:mlen], nil
	}

	// UDS path: receive returns (Header, payload, error)
	if c.session == nil {
		return protocol.Header{}, nil, protocol.ErrTruncated
	}

	hdr, payload, err := c.session.Receive(scratch)
	if err != nil {
		return protocol.Header{}, nil, protocol.ErrTruncated
	}
	return hdr, payload, nil
}

func (c *Client) maxReceiveMessageBytes() int {
	maxPayload := c.config.MaxResponsePayloadBytes
	if c.session != nil && c.session.MaxResponsePayloadBytes > 0 {
		maxPayload = c.session.MaxResponsePayloadBytes
	}
	if maxPayload == 0 {
		maxPayload = cacheResponseBufSize
	}
	return protocol.HeaderSize + int(maxPayload)
}

// Error classification helpers
func isConnectError(err error) bool {
	return errors.Is(err, posix.ErrConnect) || errors.Is(err, posix.ErrSocket)
}

func isAuthError(err error) bool {
	return errors.Is(err, posix.ErrAuthFailed)
}

func isIncompatibleError(err error) bool {
	return errors.Is(err, posix.ErrNoProfile) || errors.Is(err, posix.ErrIncompatible)
}
