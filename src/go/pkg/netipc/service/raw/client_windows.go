//go:build windows

package raw

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/internal/monotime"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// Client is an internal L2 client context bound to one service kind.
type Client struct {
	state              ClientState
	runDir             string
	serviceName        string
	expectedMethodCode uint16
	config             windows.ClientConfig

	session *windows.Session
	shm     *windows.WinShmContext

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

func newClient(runDir, serviceName string, config windows.ClientConfig, expectedMethodCode uint16) *Client {
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
		c.shm.WinShmClose()
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
	session, err := windows.ConnectTimeout(c.runDir, c.serviceName, &c.config, remaining, c.abortSignal())
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

	// Win SHM upgrade if negotiated
	if session.SelectedProfile == windows.WinShmProfileHybrid ||
		session.SelectedProfile == windows.WinShmProfileBusywait {
		// When the call deadline ends the wait first, WinSHM is not at
		// fault and stays enabled for the next connect, unless that keeps
		// happening (clientShmCallBoundFailureLimit).
		callLeft := time.Duration(c.opRemainingMs()) * time.Millisecond
		// The remaining time is rounded up, so a tie means the call deadline
		// comes first.
		callBound := callLeft <= c.shmAttachTimeout
		deadline := time.Now().Add(min(callLeft, c.shmAttachTimeout))
		for {
			shm, serr := windows.WinShmClientAttach(
				c.runDir, c.serviceName,
				c.config.AuthToken,
				session.SessionID,
				session.SelectedProfile,
			)
			if serr == nil {
				c.shm = shm
				c.shmCallBoundFailures = 0
				break
			}
			// Only objects the server has not finished yet are worth
			// waiting for.
			if !errors.Is(serr, windows.ErrWinShmOpenMapping) && !errors.Is(serr, windows.ErrWinShmOpenEvent) &&
				!errors.Is(serr, windows.ErrWinShmBadMagic) {
				break
			}
			if c.abortRequested.Load() {
				session.Close()
				return StateDisconnected
			}
			// A server that refused or died breaks the session pipe; its
			// SHM objects will never appear. Not a WinSHM problem: keep it
			// enabled for the next connect.
			// Checked before the deadline, so a peer gone at either deadline
			// is never taken for a WinSHM failure.
			if pipePeerDisconnected(session) {
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
			// WinSHM attach failed after negotiation. Close that session,
			// blacklist WinSHM for this client context, and retry baseline.
			session.Close()
			c.config.SupportedProfiles &^= winShmProfiles
			c.config.PreferredProfiles &^= winShmProfiles
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
	if c.shm != nil {
		if len(payload) > int(c.sessionMaxRequestPayloadBytes()) {
			c.noteRequestCapacity(uint32(len(payload)))
			return protocol.ErrOverflow
		}

		msgLen := protocol.HeaderSize + len(payload)
		msg := ensureClientScratch(&c.sendBuf, msgLen)

		hdr.Magic = protocol.MagicMsg
		hdr.Version = protocol.Version
		hdr.HeaderLen = protocol.HeaderLen
		hdr.PayloadLen = uint32(len(payload))

		hdr.Encode(msg[:protocol.HeaderSize])
		if len(payload) > 0 {
			copy(msg[protocol.HeaderSize:], payload)
		}

		if err := c.shm.WinShmSend(msg[:msgLen]); err != nil {
			if errors.Is(err, windows.ErrWinShmMsgTooLarge) {
				c.noteRequestCapacity(uint32(len(payload)))
				return protocol.ErrOverflow
			}
			return c.shmSendFailure()
		}
		return nil
	}

	if c.session == nil {
		return protocol.ErrTransport
	}
	if err := c.session.SendTimeout(hdr, payload, timeoutMs, c.abortSignal()); err != nil {
		if errors.Is(err, windows.ErrTimeout) {
			return protocol.ErrTimeout
		}
		if errors.Is(err, windows.ErrAborted) {
			return protocol.ErrAborted
		}
		if errors.Is(err, windows.ErrLimitExceeded) {
			c.noteRequestCapacity(uint32(len(payload)))
			return protocol.ErrOverflow
		}
		return c.pipeFailure(err, protocol.ErrTransport)
	}
	return nil
}

// pipeFailure maps a pipe send/recv failure. A broken pipe is a disconnect;
// a plain send/recv failure is one when the session probe says the server is
// gone, and a transport failure otherwise; protocol errors keep their own
// meaning.
func (c *Client) pipeFailure(err error, otherwise error) error {
	if errors.Is(err, windows.ErrDisconnected) {
		return protocol.ErrDisconnected
	}
	if errors.Is(err, windows.ErrSend) || errors.Is(err, windows.ErrRecv) {
		if c.session != nil && pipePeerDisconnected(c.session) {
			return protocol.ErrDisconnected
		}
		return protocol.ErrTransport
	}
	return otherwise
}

// shmSendFailure maps a failed SHM send: a disconnect when the session probe
// says the server is gone, a transport failure otherwise.
func (c *Client) shmSendFailure() error {
	if c.session != nil && pipePeerDisconnected(c.session) {
		return protocol.ErrDisconnected
	}
	return protocol.ErrTransport
}

// pipePeerDisconnected is a zero-wait probe of a session pipe. WinSHM sessions
// keep the pipe open for their whole life and the kernel breaks it when the
// peer exits, so a broken pipe is the dead-peer signal for both directions.
func pipePeerDisconnected(session *windows.Session) bool {
	_, err := session.WaitReadable(0)
	return errors.Is(err, windows.ErrDisconnected)
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

			mlen, err := c.shm.WinShmReceive(scratch, waitMs)
			if err != nil {
				if errors.Is(err, windows.ErrWinShmTimeout) {
					// Only an idle slice pays for the probe.
					if c.session != nil && pipePeerDisconnected(c.session) {
						return protocol.Header{}, nil, protocol.ErrDisconnected
					}
					continue
				}
				if errors.Is(err, windows.ErrWinShmDisconnected) {
					return protocol.Header{}, nil, protocol.ErrDisconnected
				}
				if errors.Is(err, windows.ErrWinShmMsgTooLarge) {
					return protocol.Header{}, nil, protocol.ErrOverflow
				}
				// A zero-length or corrupt slot is a truncated message.
				if errors.Is(err, windows.ErrWinShmBadHeader) {
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

	if c.session == nil {
		return protocol.Header{}, nil, protocol.ErrTransport
	}

	hdr, payload, err := c.session.ReceiveTimeout(scratch, timeoutMs, abortCh)
	if err != nil {
		if errors.Is(err, windows.ErrTimeout) {
			return protocol.Header{}, nil, protocol.ErrTimeout
		}
		if errors.Is(err, windows.ErrAborted) {
			return protocol.Header{}, nil, protocol.ErrAborted
		}
		if errors.Is(err, windows.ErrLimitExceeded) {
			return protocol.Header{}, nil, protocol.ErrOverflow
		}
		return protocol.Header{}, nil, c.pipeFailure(err, protocol.ErrTruncated)
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
	return errors.Is(err, windows.ErrConnect) || errors.Is(err, windows.ErrCreatePipe)
}

func isAuthError(err error) bool {
	return errors.Is(err, windows.ErrAuthFailed) || errors.Is(err, windows.ErrUntrustedServer)
}

func isIncompatibleError(err error) bool {
	return errors.Is(err, windows.ErrNoProfile) || errors.Is(err, windows.ErrIncompatible)
}
