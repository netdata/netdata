// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var (
	ErrFrameOwnerBusy        = errors.New("jobmgr frame owner: busy")
	ErrFrameOwnerPoisoned    = errors.New("jobmgr frame owner: poisoned")
	ErrPreparedFrameConsumed = errors.New("jobmgr frame owner: prepared frame consumed")
)

type PreparedFrame struct {
	state *preparedFunctionFrame
}

type preparedFunctionFrame struct {
	mu           sync.Mutex
	consumed     bool
	uid          string
	result       SealedResult
	expiry       int64
	encodedBytes int
}

type preparedFunctionFrameContent struct {
	uid          string
	result       SealedResult
	expiry       int64
	encodedBytes int
}

type PreparedProtocolFrame struct {
	state *preparedProtocolFrame
}

type preparedProtocolFrame struct {
	mu       sync.Mutex
	consumed bool
	payload  []byte
}

type FrameCensus struct {
	Poisoned       bool
	Busy           bool
	PendingControl bool
	RetainedBytes  int
	Commits        uint64
}

type FrameOwner struct {
	stateMu        sync.Mutex
	available      *sync.Cond
	writer         io.Writer
	busy           bool
	pendingControl bool
	poisoned       bool
	retained       []byte
	commits        uint64
	onControlReady func()
	controlBuffer  [ControlFrameBytes]byte
}

func NewFrameOwner(writer io.Writer) (*FrameOwner, error) {
	if writer == nil {
		return nil, errors.New("jobmgr frame owner: nil writer")
	}
	owner := &FrameOwner{writer: writer}
	owner.available = sync.NewCond(&owner.stateMu)
	return owner, nil
}

func (owner *FrameOwner) BindControlReady(notify func()) error {
	if owner == nil || notify == nil {
		return errors.New("jobmgr frame owner: invalid control-ready binding")
	}
	owner.stateMu.Lock()
	if owner.onControlReady != nil {
		owner.stateMu.Unlock()
		return errors.New("jobmgr frame owner: control-ready notifier already bound")
	}
	owner.onControlReady = notify
	pending := owner.pendingControl && !owner.busy
	owner.stateMu.Unlock()
	if pending {
		notify()
	}
	return nil
}

func PrepareFrame(uid string, result SealedResult, expiry int64) (PreparedFrame, error) {
	if err := result.validate(); err != nil {
		return PreparedFrame{}, err
	}
	encodedBytes, _, err := functionFrameSize(uid, result.status, result.contentType, expiry, result.payloadBytes)
	if err != nil {
		return PreparedFrame{}, err
	}
	return PreparedFrame{state: &preparedFunctionFrame{uid: uid, result: result, expiry: expiry, encodedBytes: encodedBytes}}, nil
}

func NewControlResult(status ControlStatus) (SealedResult, error) {
	if err := (ControlFramePlan{UID: "validation", Status: status, Expiry: 1}).Validate(); err != nil {
		return SealedResult{}, err
	}
	return NewSealedResult(int(status), "application/json", controlPayload(status))
}

func (owner *FrameOwner) Commit(frame PreparedFrame) error {
	content, err := frame.take()
	if err != nil {
		return err
	}
	payload, err := appendPreparedFrame(make([]byte, 0, content.encodedBytes), content)
	if err != nil {
		return err
	}
	return owner.commitOrdinary(payload)
}

func appendPreparedFrame(dst []byte, frame preparedFunctionFrameContent) ([]byte, error) {
	if frame.encodedBytes <= 0 {
		return dst, errors.New("jobmgr frame owner: unprepared frame")
	}
	start := len(dst)
	dst = fmt.Appendf(dst, "FUNCTION_RESULT_BEGIN %s %d %s %d\n", frame.uid, frame.result.status, frame.result.contentType, frame.expiry)
	var err error
	dst, err = frame.result.appendPayload(dst)
	if err != nil {
		return dst, err
	}
	if frame.result.payloadBytes > 0 {
		dst = append(dst, '\n')
	}
	dst = append(dst, "FUNCTION_RESULT_END\n\n"...)
	if len(dst)-start != frame.encodedBytes {
		return dst, errors.New("jobmgr frame owner: prepared Size/Append divergence")
	}
	return dst, nil
}

func (frame PreparedFrame) encodedSize() (int, error) {
	if frame.state == nil {
		return 0, errors.New("jobmgr frame owner: unprepared frame")
	}
	frame.state.mu.Lock()
	defer frame.state.mu.Unlock()
	if frame.state.consumed {
		return 0, ErrPreparedFrameConsumed
	}
	return frame.state.encodedBytes, nil
}

func (frame PreparedFrame) take() (preparedFunctionFrameContent, error) {
	if frame.state == nil {
		return preparedFunctionFrameContent{}, errors.New("jobmgr frame owner: unprepared frame")
	}
	frame.state.mu.Lock()
	defer frame.state.mu.Unlock()
	if frame.state.consumed {
		return preparedFunctionFrameContent{}, ErrPreparedFrameConsumed
	}
	frame.state.consumed = true
	content := preparedFunctionFrameContent{
		uid: frame.state.uid, result: frame.state.result, expiry: frame.state.expiry, encodedBytes: frame.state.encodedBytes,
	}
	frame.state.result = SealedResult{}
	return content, nil
}

func PrepareProtocolFrame(payload []byte) (PreparedProtocolFrame, error) {
	if len(payload) == 0 || len(payload) > MaximumOtherFrameBytes {
		return PreparedProtocolFrame{}, errors.New("jobmgr frame owner: invalid protocol frame size")
	}
	return PreparedProtocolFrame{state: &preparedProtocolFrame{payload: append([]byte(nil), payload...)}}, nil
}

func (frame PreparedProtocolFrame) Abort() error {
	_, err := frame.take()
	return err
}

func (frame PreparedProtocolFrame) take() ([]byte, error) {
	if frame.state == nil {
		return nil, errors.New("jobmgr frame owner: unprepared protocol frame")
	}
	frame.state.mu.Lock()
	defer frame.state.mu.Unlock()
	if frame.state.consumed {
		return nil, ErrPreparedFrameConsumed
	}
	frame.state.consumed = true
	payload := frame.state.payload
	frame.state.payload = nil
	return payload, nil
}

func (owner *FrameOwner) CommitProtocolFrame(payload []byte) error {
	frame, err := PrepareProtocolFrame(payload)
	if err != nil {
		return err
	}
	return owner.CommitPreparedProtocolFrame(frame)
}

func (owner *FrameOwner) CommitPreparedProtocolFrame(frame PreparedProtocolFrame) error {
	payload, err := frame.take()
	if err != nil {
		return err
	}
	return owner.commitOrdinary(payload)
}

func (owner *FrameOwner) commitOrdinary(payload []byte) error {
	owner.stateMu.Lock()
	for (owner.busy || owner.pendingControl) && !owner.poisoned {
		owner.available.Wait()
	}
	if owner.poisoned {
		owner.stateMu.Unlock()
		return ErrFrameOwnerPoisoned
	}
	owner.busy = true
	owner.stateMu.Unlock()
	return owner.writeAndRelease(payload, false)
}

func (owner *FrameOwner) TryCommitControl(plan ControlFramePlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	owner.stateMu.Lock()
	if owner.poisoned {
		owner.stateMu.Unlock()
		return ErrFrameOwnerPoisoned
	}
	owner.pendingControl = true
	if owner.busy {
		owner.stateMu.Unlock()
		return ErrFrameOwnerBusy
	}
	owner.busy = true
	owner.pendingControl = false
	owner.stateMu.Unlock()

	payload := controlPayload(plan.Status)
	encoded, err := encodeResult(owner.controlBuffer[:0], plan.UID, int(plan.Status), "application/json", plan.Expiry, payload, ControlFrameBytes, 0, 0)
	if err != nil {
		owner.poison(encoded)
		return err
	}
	return owner.writeAndRelease(encoded, true)
}

func (owner *FrameOwner) Census() FrameCensus {
	owner.stateMu.Lock()
	defer owner.stateMu.Unlock()
	return FrameCensus{Poisoned: owner.poisoned, Busy: owner.busy, PendingControl: owner.pendingControl, RetainedBytes: len(owner.retained), Commits: owner.commits}
}

func (owner *FrameOwner) writeAndRelease(payload []byte, control bool) error {
	count, err := owner.writer.Write(payload)
	if err != nil || count != len(payload) {
		if err == nil {
			err = io.ErrShortWrite
		}
		owner.poison(payload)
		return fmt.Errorf("jobmgr frame owner: commit: %w", err)
	}
	owner.stateMu.Lock()
	owner.busy = false
	owner.commits++
	pending := owner.pendingControl
	notify := owner.onControlReady
	owner.available.Broadcast()
	owner.stateMu.Unlock()
	if pending && notify != nil {
		notify()
	}
	_ = control
	return nil
}

func (owner *FrameOwner) poison(payload []byte) {
	owner.stateMu.Lock()
	owner.poisoned = true
	owner.busy = false
	owner.retained = payload
	owner.available.Broadcast()
	owner.stateMu.Unlock()
}

func encodeResult(dst []byte, uid string, status int, contentType string, expiry int64, payload []byte, frameLimit, envelopeLimit, payloadLimit int) ([]byte, error) {
	encodedBytes, _, err := resultFrameSize(uid, status, contentType, expiry, len(payload), frameLimit, envelopeLimit, payloadLimit)
	if err != nil {
		return dst, err
	}
	start := len(dst)
	if cap(dst)-len(dst) < encodedBytes {
		grown := make([]byte, len(dst), len(dst)+encodedBytes)
		copy(grown, dst)
		dst = grown
	}
	dst = fmt.Appendf(dst, "FUNCTION_RESULT_BEGIN %s %d %s %d\n", uid, status, contentType, expiry)
	dst = append(dst, payload...)
	if len(payload) > 0 {
		dst = append(dst, '\n')
	}
	dst = append(dst, "FUNCTION_RESULT_END\n\n"...)
	if len(dst)-start != encodedBytes {
		return dst, errors.New("jobmgr frame owner: Size/Append divergence")
	}
	return dst, nil
}

func functionFrameSize(uid string, status int, contentType string, expiry int64, payloadBytes int) (frameBytes, envelopeBytes int, err error) {
	return resultFrameSize(uid, status, contentType, expiry, payloadBytes, MaximumFunctionFrameBytes, FunctionEnvelopeBytes, FunctionPayloadBytes)
}

func resultFrameSize(uid string, status int, contentType string, expiry int64, payloadBytes, frameLimit, envelopeLimit, payloadLimit int) (frameBytes, envelopeBytes int, err error) {
	if uid == "" || strings.ContainsAny(uid, " \t\r\n\x00") || contentType == "" || strings.ContainsAny(contentType, " \t\r\n\x00") || status < 100 || status > 599 || expiry <= 0 {
		return 0, 0, errors.New("jobmgr frame owner: unsafe frame header")
	}
	if payloadBytes < 0 || frameLimit <= 0 || envelopeLimit < 0 || payloadLimit < 0 {
		return 0, 0, errors.New("jobmgr frame owner: invalid frame size policy")
	}
	payloadLF := 0
	if payloadBytes > 0 {
		payloadLF = 1
	}
	headerBytes, ok := checkedSizeSum(
		len("FUNCTION_RESULT_BEGIN "), len(uid), 1, decimalBytes(uint64(status)), 1,
		len(contentType), 1, decimalBytes(uint64(expiry)), 1,
	)
	if !ok {
		return 0, 0, fmt.Errorf("%w: header size overflow", ErrFunctionResultTooLarge)
	}
	envelopeBytes, ok = checkedSizeSum(headerBytes, payloadLF, len("FUNCTION_RESULT_END\n\n"))
	if !ok {
		return 0, 0, fmt.Errorf("%w: envelope size overflow", ErrFunctionResultTooLarge)
	}
	deferredBytes, ok := checkedSizeSum(payloadBytes, payloadLF)
	if !ok {
		return 0, 0, fmt.Errorf("%w: payload size overflow", ErrFunctionResultTooLarge)
	}
	frameBytes, ok = checkedSizeSum(headerBytes, payloadBytes, payloadLF, len("FUNCTION_RESULT_END\n\n"))
	if !ok {
		return 0, 0, fmt.Errorf("%w: frame size overflow", ErrFunctionResultTooLarge)
	}
	if (payloadLimit > 0 && deferredBytes > payloadLimit) || (envelopeLimit > 0 && envelopeBytes > envelopeLimit) || frameBytes > frameLimit {
		return 0, 0, fmt.Errorf("%w: payload=%d envelope=%d frame=%d", ErrFunctionResultTooLarge, deferredBytes, envelopeBytes, frameBytes)
	}
	return frameBytes, envelopeBytes, nil
}

func checkedSizeSum(values ...int) (int, bool) {
	total := 0
	maximum := int(^uint(0) >> 1)
	for _, value := range values {
		if value < 0 || total > maximum-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func decimalBytes(value uint64) int {
	bytes := 1
	for value >= 10 {
		value /= 10
		bytes++
	}
	return bytes
}

func controlPayload(status ControlStatus) []byte {
	switch status {
	case ControlBadRequest:
		return []byte(`{"errorMessage":"Bad request.","status":400}`)
	case ControlNotFound:
		return []byte(`{"errorMessage":"Not found.","status":404}`)
	case ControlPayloadTooLarge:
		return []byte(`{"errorMessage":"Payload too large.","status":413}`)
	case ControlInternal:
		return []byte(`{"errorMessage":"Internal error.","status":500}`)
	case ControlCancelled:
		return []byte(`{"errorMessage":"Request cancelled.","status":499}`)
	case ControlDeadline:
		return []byte(`{"errorMessage":"Deadline exceeded.","status":504}`)
	default:
		return []byte(`{"errorMessage":"Service unavailable.","status":503}`)
	}
}

func ExpiryAt(now time.Time) int64 {
	return now.Unix()
}
