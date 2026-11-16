//go:build unix

package raw

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
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
	// Request header of the call in progress: its address goes to the
	// transport, so a per-call variable would be a heap allocation per call.
	requestHdr protocol.Header
	// Views returned by the typed calls: valid until the next typed call, and
	// kept here so that returning their address does not allocate.
	snapshotView      protocol.CgroupsResponseView
	stringReverseView protocol.StringReverseView

	// Stats
	connectCount   uint32
	reconnectCount uint32
	callCount      uint32
	errorCount     uint32

	callTimeoutMs                 uint32
	maxLogicalLookupItems         uint32
	maxLogicalLookupSubcalls      uint32
	maxLogicalLookupResponseBytes uint32
	abortRequested                atomic.Bool
	abortMu                       sync.Mutex
	abortCh                       chan struct{}
	// shmAttachTimeout bounds the SHM attach retry after negotiation;
	// tests shorten it per instance.
	shmAttachTimeout time.Duration
	// shmCallBoundFailures counts consecutive SHM attaches the call
	// deadline ended with the session still connected.
	shmCallBoundFailures int
	// opDeadline bounds the call or refresh in progress, in monotime
	// nanoseconds; zero outside one.
	opDeadline int64
}

func newClient(runDir, serviceName string, config posix.ClientConfig, expectedMethodCode uint16) *Client {
	client := &Client{
		state:              StateDisconnected,
		runDir:             runDir,
		serviceName:        serviceName,
		expectedMethodCode: expectedMethodCode,
		config:             config,
		callTimeoutMs:      ClientCallTimeoutDefaultMs,
		abortCh:            make(chan struct{}),
		shmAttachTimeout:   clientShmAttachRetryTimeout,
	}
	client.SetLookupLogicalConfig(LookupLogicalConfig{})
	return client
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
	remaining := c.opRemainingMs()
	if remaining == 0 || c.abortRequested.Load() {
		return StateDisconnected
	}
	session, err := posix.ConnectTimeout(c.runDir, c.serviceName, &c.config, remaining, c.abortSignal())
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
		// When the call deadline ends the wait first, SHM is not at fault
		// and stays enabled for the next connect, unless that keeps
		// happening (clientShmCallBoundFailureLimit).
		callLeft := time.Duration(c.opRemainingMs()) * time.Millisecond
		// The remaining time is rounded up, so a tie means the call deadline
		// comes first.
		callBound := callLeft <= c.shmAttachTimeout
		deadline := time.Now().Add(min(callLeft, c.shmAttachTimeout))
		for {
			shm, serr := posix.ShmClientAttach(c.runDir, c.serviceName, session.SessionID)
			if serr == nil {
				c.shm = shm
				c.shmCallBoundFailures = 0
				break
			}
			// Only a region the server has not finished yet is worth
			// waiting for.
			if !errors.Is(serr, posix.ErrShmNotReady) && !errors.Is(serr, posix.ErrShmOpen) &&
				!errors.Is(serr, posix.ErrShmBadMagic) {
				break
			}
			if c.abortRequested.Load() {
				session.Close()
				return StateDisconnected
			}
			// A server that refused or died closes the session socket; its
			// SHM region will never appear. Not an SHM problem: keep SHM
			// enabled for the next connect.
			// Checked before the deadline, so a peer gone at either deadline
			// is never taken for an SHM failure.
			if peerDisconnected(session.Fd()) {
				session.Close()
				return StateDisconnected
			}
			if !time.Now().Before(deadline) {
				if callBound {
					c.shmCallBoundFailures++
					if c.shmCallBoundFailures < clientShmCallBoundFailureLimit {
						session.Close()
						return StateDisconnected
					}
				}
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

// transportSend sends via the active transport. The baseline send ends at
// timeoutMs (0 = no deadline) or on abort; the SHM send never waits for the
// peer.
func (c *Client) transportSend(hdr *protocol.Header, payload []byte, timeoutMs uint32) error {
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
			return c.shmSendFailure()
		}
		return nil
	}

	// UDS path
	if c.session == nil {
		return protocol.ErrTransport
	}
	if err := c.session.SendTimeout(hdr, payload, timeoutMs, c.abortSignal()); err != nil {
		if errors.Is(err, posix.ErrTimeout) {
			return protocol.ErrTimeout
		}
		if errors.Is(err, posix.ErrAborted) {
			return protocol.ErrAborted
		}
		if errors.Is(err, posix.ErrLimitExceeded) {
			c.noteRequestCapacity(payloadLen)
			return protocol.ErrOverflow
		}
		return c.socketFailure(err, protocol.ErrTransport)
	}
	return nil
}

// socketFailure maps a socket send/recv failure. The transport reports EOF,
// EPIPE and ECONNRESET as plain send/recv failures, so the session probe tells
// a gone server apart from any other I/O failure; protocol errors keep their
// own meaning.
func (c *Client) socketFailure(err error, otherwise error) error {
	if errors.Is(err, posix.ErrSend) || errors.Is(err, posix.ErrRecv) {
		if c.session != nil && peerDisconnected(c.session.Fd()) {
			return protocol.ErrDisconnected
		}
		return protocol.ErrTransport
	}
	return otherwise
}

// shmSendFailure maps a failed SHM send: a disconnect when the session probe
// says the server is gone, a transport failure otherwise.
func (c *Client) shmSendFailure() error {
	if c.session != nil && peerDisconnected(c.session.Fd()) {
		return protocol.ErrDisconnected
	}
	return protocol.ErrTransport
}

func (c *Client) transportReceive() (protocol.Header, []byte, error) {
	return c.transportReceiveWithControl(c.resolvedCallTimeout(0), c.abortSignal())
}

func (c *Client) transportReceiveWithControl(timeoutMs uint32, abortCh <-chan struct{}) (protocol.Header, []byte, error) {
	scratch := ensureClientScratch(&c.transportBuf, c.maxReceiveMessageBytes())

	if c.shm != nil {
		// One monotonic read per slice, as in C.
		deadline := monotime.After(time.Duration(timeoutMs) * time.Millisecond)
		for {
			select {
			case <-abortCh:
				return protocol.Header{}, nil, protocol.ErrAborted
			default:
			}

			waitMs := clientAbortPollMs
			if timeoutMs != 0 {
				remaining := time.Duration(deadline - monotime.Now())
				if remaining <= 0 {
					return protocol.Header{}, nil, protocol.ErrTimeout
				}
				waitMs = boundedClientWaitMs(remaining, waitMs)
			}

			mlen, err := c.shm.ShmReceive(scratch, waitMs)
			if err != nil {
				if errors.Is(err, posix.ErrShmTimeout) {
					// Only an idle slice pays for the probe.
					if c.session != nil && peerDisconnected(c.session.Fd()) {
						return protocol.Header{}, nil, protocol.ErrDisconnected
					}
					continue
				}
				if errors.Is(err, posix.ErrShmMsgTooLarge) {
					return protocol.Header{}, nil, protocol.ErrOverflow
				}
				// A zero-length or corrupt slot is a truncated message.
				if errors.Is(err, posix.ErrShmBadHeader) {
					return protocol.Header{}, nil, protocol.ErrTruncated
				}
				return protocol.Header{}, nil, protocol.ErrTransport
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
	}

	// UDS path: receive returns (Header, payload, error)
	if c.session == nil {
		return protocol.Header{}, nil, protocol.ErrTransport
	}

	hdr, payload, err := c.session.ReceiveTimeout(scratch, timeoutMs, abortCh)
	if err != nil {
		if errors.Is(err, posix.ErrTimeout) {
			return protocol.Header{}, nil, protocol.ErrTimeout
		}
		if errors.Is(err, posix.ErrAborted) {
			return protocol.Header{}, nil, protocol.ErrAborted
		}
		if errors.Is(err, posix.ErrLimitExceeded) {
			return protocol.Header{}, nil, protocol.ErrOverflow
		}
		return protocol.Header{}, nil, c.socketFailure(err, protocol.ErrTruncated)
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
