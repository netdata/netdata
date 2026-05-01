//go:build unix

package raw

import (
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

const clientShmAttachRetryInterval = 5 * time.Millisecond
const clientShmAttachRetryTimeout = 5 * time.Second

// ---------------------------------------------------------------------------
//  Client context
// ---------------------------------------------------------------------------

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

// NewSnapshotClient creates a raw client bound to the cgroups-snapshot service kind.
func NewSnapshotClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodCgroupsSnapshot)
}

// NewIncrementClient creates a raw client bound to the increment service kind.
func NewIncrementClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodIncrement)
}

// NewStringReverseClient creates a raw client bound to the string-reverse service kind.
func NewStringReverseClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodStringReverse)
}

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
	grown := nextPowerOf2U32(payloadLen)
	if grown > protocol.MaxPayloadCap {
		grown = protocol.MaxPayloadCap
	}
	if grown > c.config.MaxRequestPayloadBytes {
		c.config.MaxRequestPayloadBytes = grown
	}
}

func (c *Client) noteResponseCapacity(payloadLen uint32) {
	grown := nextPowerOf2U32(payloadLen)
	if grown > protocol.MaxPayloadCap {
		grown = protocol.MaxPayloadCap
	}
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

// CallSnapshot performs a blocking typed cgroups snapshot call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallSnapshot() (*protocol.CgroupsResponseView, error) {
	if err := c.validateMethod(protocol.MethodCgroupsSnapshot); err != nil {
		return nil, err
	}

	var result *protocol.CgroupsResponseView

	err := c.callWithRetry(func() error {
		req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
		var reqBuf [4]byte
		if req.Encode(reqBuf[:]) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCall(protocol.MethodCgroupsSnapshot, reqBuf[:])
		if rerr != nil {
			return rerr
		}

		view, derr := protocol.DecodeCgroupsResponse(payload)
		if derr != nil {
			return derr
		}
		result = &view
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallIncrement performs a blocking INCREMENT call.
// Sends requestValue, returns the server's response value.
func (c *Client) CallIncrement(requestValue uint64) (uint64, error) {
	if err := c.validateMethod(protocol.MethodIncrement); err != nil {
		return 0, err
	}

	var result uint64

	err := c.callWithRetry(func() error {
		var reqBuf [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(requestValue, reqBuf[:]) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCall(protocol.MethodIncrement, reqBuf[:])
		if rerr != nil {
			return rerr
		}

		val, derr := protocol.IncrementDecode(payload)
		if derr != nil {
			return derr
		}
		result = val
		return nil
	})
	return result, err
}

// CallStringReverse performs a blocking STRING_REVERSE call.
// The returned view is valid until the next typed call on this client.
func (c *Client) CallStringReverse(requestStr string) (*protocol.StringReverseView, error) {
	if err := c.validateMethod(protocol.MethodStringReverse); err != nil {
		return nil, err
	}

	var result *protocol.StringReverseView

	err := c.callWithRetry(func() error {
		reqBuf := ensureClientScratch(&c.requestBuf, protocol.StringReverseHdrSize+len(requestStr)+1)
		if protocol.StringReverseEncode(requestStr, reqBuf) == 0 {
			return protocol.ErrTruncated
		}

		_, payload, rerr := c.doRawCall(protocol.MethodStringReverse, reqBuf)
		if rerr != nil {
			return rerr
		}

		view, derr := protocol.StringReverseDecode(payload)
		if derr != nil {
			return derr
		}
		result = &view
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallIncrementBatch performs a blocking batch INCREMENT call.
// Sends multiple values, returns the server's response values.
func (c *Client) CallIncrementBatch(values []uint64) ([]uint64, error) {
	if err := c.validateMethod(protocol.MethodIncrement); err != nil {
		return nil, err
	}

	if len(values) == 0 {
		return nil, nil
	}

	var results []uint64
	itemCount := uint32(len(values))

	err := c.callWithRetry(func() error {
		// Build batch request payload
		batchBufSize := protocol.Align8(int(itemCount)*8) + int(itemCount)*protocol.IncrementPayloadSize + int(itemCount)*protocol.Alignment
		batchBuf := ensureClientScratch(&c.requestBuf, batchBufSize)
		bb := protocol.NewBatchBuilder(batchBuf, itemCount)

		for _, v := range values {
			var item [protocol.IncrementPayloadSize]byte
			if protocol.IncrementEncode(v, item[:]) == 0 {
				return protocol.ErrTruncated
			}
			if err := bb.Add(item[:]); err != nil {
				return err
			}
		}

		totalPayloadLen, _ := bb.Finish()
		reqPayload := batchBuf[:totalPayloadLen]

		// Build and send header with batch flags
		hdr := protocol.Header{
			Kind:            protocol.KindRequest,
			Code:            protocol.MethodIncrement,
			Flags:           protocol.FlagBatch,
			ItemCount:       itemCount,
			MessageID:       uint64(c.callCount) + 1,
			TransportStatus: protocol.StatusOK,
		}

		if err := c.transportSend(&hdr, reqPayload); err != nil {
			return err
		}

		// Receive response
		respHdr, respPayload, err := c.transportReceive()
		if err != nil {
			return err
		}

		if respHdr.Kind != protocol.KindResponse {
			return protocol.ErrBadKind
		}
		if respHdr.Code != protocol.MethodIncrement {
			return protocol.ErrBadLayout
		}
		if respHdr.MessageID != hdr.MessageID {
			return protocol.ErrBadLayout
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
			return protocol.ErrOverflow
		default:
			return protocol.ErrBadLayout
		}
		if respHdr.Flags&protocol.FlagBatch == 0 || respHdr.ItemCount != itemCount {
			return protocol.ErrBadItemCount
		}

		// Extract each response item
		out := make([]uint64, itemCount)
		for i := uint32(0); i < itemCount; i++ {
			itemData, gerr := protocol.BatchItemGet(respPayload, itemCount, i)
			if gerr != nil {
				return gerr
			}
			val, derr := protocol.IncrementDecode(itemData)
			if derr != nil {
				return derr
			}
			out[i] = val
		}
		results = out
		return nil
	})
	return results, err
}

// Close tears down the connection and releases resources.
func (c *Client) Close() {
	c.disconnect()
	c.state = StateDisconnected
}

// ------------------------------------------------------------------
//  Internal helpers
// ------------------------------------------------------------------

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

		if err := c.shm.ShmSend(msg[:msgLen]); err != nil {
			if errors.Is(err, posix.ErrShmMsgTooLarge) {
				c.noteRequestCapacity(uint32(len(payload)))
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
			c.noteRequestCapacity(uint32(len(payload)))
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

// ---------------------------------------------------------------------------
//  Managed server
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
//  Internal: poll helper (raw syscall, pure Go, no cgo)
// ---------------------------------------------------------------------------

// poll constants (not exported by Go's syscall package)
const (
	_POLLIN   = 0x0001
	_POLLERR  = 0x0008
	_POLLHUP  = 0x0010
	_POLLNVAL = 0x0020
)

// pollfd matches struct pollfd from <poll.h>.
type pollfd struct {
	fd      int32
	events  int16
	revents int16
}

// pollFd polls a file descriptor for readability with a timeout in ms.
// Returns: 1 = data ready, 0 = timeout, -1 = error/hangup.
func pollFd(fd int, timeoutMs int) int {
	pfd := pollfd{
		fd:     int32(fd),
		events: _POLLIN,
	}

	r, _, errno := syscall.Syscall(
		syscall.SYS_POLL,
		uintptr(unsafe.Pointer(&pfd)),
		1,
		uintptr(timeoutMs),
	)

	n := int(r)
	if n < 0 {
		if errno == syscall.EINTR {
			return 0
		}
		return -1
	}

	if n == 0 {
		return 0
	}

	if pfd.revents&(_POLLERR|_POLLHUP|_POLLNVAL) != 0 {
		return -1
	}

	if pfd.revents&_POLLIN != 0 {
		return 1
	}

	return 0
}

// Suppress unused import warnings.
var _ = binary.NativeEndian
