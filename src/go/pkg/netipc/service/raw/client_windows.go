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

	connectCount   uint32
	reconnectCount uint32
	callCount      uint32
	errorCount     uint32

	callTimeoutMs  uint32
	abortRequested atomic.Bool
	abortMu        sync.Mutex
	abortCh        chan struct{}
}

func newClient(runDir, serviceName string, config windows.ClientConfig, expectedMethodCode uint16) *Client {
	return &Client{
		state:              StateDisconnected,
		runDir:             runDir,
		serviceName:        serviceName,
		expectedMethodCode: expectedMethodCode,
		config:             config,
		callTimeoutMs:      ClientCallTimeoutDefaultMs,
		abortCh:            make(chan struct{}),
	}
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
	session, err := windows.Connect(c.runDir, c.serviceName, &c.config)
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
		deadline := time.Now().Add(clientShmAttachRetryTimeout)
		for {
			shm, serr := windows.WinShmClientAttach(
				c.runDir, c.serviceName,
				c.config.AuthToken,
				session.SessionID,
				session.SelectedProfile,
			)
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

func (c *Client) transportSend(hdr *protocol.Header, payload []byte) error {
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
			return protocol.ErrTruncated
		}
		return nil
	}

	if c.session == nil {
		return protocol.ErrTruncated
	}
	if err := c.session.Send(hdr, payload); err != nil {
		if errors.Is(err, windows.ErrLimitExceeded) {
			c.noteRequestCapacity(uint32(len(payload)))
			return protocol.ErrOverflow
		}
		return protocol.ErrTruncated
	}
	return nil
}

func (c *Client) transportReceive() (protocol.Header, []byte, error) {
	return c.transportReceiveWithControl(c.resolvedCallTimeout(0), c.abortSignal())
}

func (c *Client) transportReceiveWithControl(timeoutMs uint32, abortCh <-chan struct{}) (protocol.Header, []byte, error) {
	scratch := ensureClientScratch(&c.transportBuf, c.maxReceiveMessageBytes())

	if c.shm != nil {
		deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
		for {
			select {
			case <-abortCh:
				return protocol.Header{}, nil, protocol.ErrAborted
			default:
			}

			if timeoutMs != 0 && !time.Now().Before(deadline) {
				return protocol.Header{}, nil, protocol.ErrTimeout
			}

			waitMs := clientAbortPollMs
			if timeoutMs != 0 {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return protocol.Header{}, nil, protocol.ErrTimeout
				}
				waitMs = boundedClientWaitMs(remaining, waitMs)
			}

			mlen, err := c.shm.WinShmReceive(scratch, waitMs)
			if err != nil {
				if errors.Is(err, windows.ErrWinShmTimeout) {
					continue
				}
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
	}

	if c.session == nil {
		return protocol.Header{}, nil, protocol.ErrTruncated
	}

	hdr, payload, err := c.session.ReceiveTimeout(scratch, timeoutMs, abortCh)
	if err != nil {
		if errors.Is(err, windows.ErrTimeout) {
			return protocol.Header{}, nil, protocol.ErrTimeout
		}
		if errors.Is(err, windows.ErrAborted) {
			return protocol.Header{}, nil, protocol.ErrAborted
		}
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
	return errors.Is(err, windows.ErrConnect) || errors.Is(err, windows.ErrCreatePipe)
}

func isAuthError(err error) bool {
	return errors.Is(err, windows.ErrAuthFailed)
}

func isIncompatibleError(err error) bool {
	return errors.Is(err, windows.ErrNoProfile) || errors.Is(err, windows.ErrIncompatible)
}
