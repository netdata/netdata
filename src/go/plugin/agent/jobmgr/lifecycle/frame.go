// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"slices"
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
}

type FrameOwner struct {
	stateMu         sync.Mutex
	available       *sync.Cond
	writer          io.Writer
	busy            bool
	pendingControl  bool
	poisoned        bool
	poisonErr       error
	retained        []byte
	onControlReady  func()
	onPoisoned      func(error)
	runtimeObserver RuntimeObserver
	runBinding      uint64
	controlBuffer   [ControlFrameBytes]byte
}

// ProtocolTransaction is the in-memory half of one serialized protocol frame.
// FrameOwner commits it only after the complete frame is written and aborts it
// on every other terminal path.
type ProtocolTransaction interface {
	Commit() error
	Abort() error
}

func NewFrameOwner(writer io.Writer) (*FrameOwner, error) {
	if writer == nil {
		return nil, errors.New("jobmgr frame owner: nil writer")
	}
	owner := &FrameOwner{writer: writer}
	owner.available = sync.NewCond(&owner.stateMu)
	return owner, nil
}

// BindRunNotifications installs one generation-scoped notification lease.
// Process composition releases the retired generation before binding its
// successor to this process-global FrameOwner.
func (fo *FrameOwner) BindRunNotifications(
	generation uint64,
	controlReady func(),
	poisoned func(error),
	observer RuntimeObserver,
) error {
	if fo == nil || generation == 0 || controlReady == nil ||
		poisoned == nil {
		return errors.New("jobmgr frame owner: invalid run notification binding")
	}
	fo.stateMu.Lock()
	if fo.runBinding != 0 ||
		fo.onControlReady != nil ||
		fo.onPoisoned != nil {
		fo.stateMu.Unlock()
		return errors.New("jobmgr frame owner: notifications already bound")
	}
	fo.runBinding = generation
	fo.onControlReady = controlReady
	fo.onPoisoned = poisoned
	fo.runtimeObserver = observer
	pending := fo.pendingControl && !fo.busy
	poisonErr := fo.poisonErr
	fo.stateMu.Unlock()
	if pending {
		controlReady()
	}
	if poisonErr != nil {
		poisoned(poisonErr)
	}
	return nil
}

func (fo *FrameOwner) ReleaseRunNotifications(generation uint64) error {
	if fo == nil || generation == 0 {
		return errors.New("jobmgr frame owner: invalid run notification release")
	}
	fo.stateMu.Lock()
	defer fo.stateMu.Unlock()
	if fo.runBinding != generation ||
		fo.onControlReady == nil ||
		fo.onPoisoned == nil {
		return errors.New("jobmgr frame owner: stale run notification release")
	}
	fo.runBinding = 0
	fo.onControlReady = nil
	fo.onPoisoned = nil
	fo.runtimeObserver = nil
	return nil
}

func PrepareFrame(uid string, result SealedResult, expiry int64) (PreparedFrame, error) {
	if err := result.validate(); err != nil {
		return PreparedFrame{}, err
	}
	encodedBytes, _, err := functionFrameSize(uid, result.status, result.contentType, expiry, len(result.payload))
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

func (fo *FrameOwner) Commit(frame PreparedFrame) error {
	content, err := frame.take()
	if err != nil {
		return err
	}
	payload, err := appendPreparedFrame(make([]byte, 0, content.encodedBytes), content)
	if err != nil {
		return err
	}
	return fo.commitOrdinary(payload)
}

func appendPreparedFrame(dst []byte, frame preparedFunctionFrameContent) ([]byte, error) {
	if frame.encodedBytes <= 0 {
		return dst, errors.New("jobmgr frame owner: unprepared frame")
	}
	start := len(dst)
	dst = fmt.Appendf(dst, "FUNCTION_RESULT_BEGIN %s %d %s %d\n", frame.uid, frame.result.status, frame.result.contentType, frame.expiry)
	dst = append(dst, frame.result.payload...)
	if len(frame.result.payload) > 0 {
		dst = append(dst, '\n')
	}
	dst = append(dst, "FUNCTION_RESULT_END\n\n"...)
	if len(dst)-start != frame.encodedBytes {
		return dst, errors.New("jobmgr frame owner: prepared Size/Append divergence")
	}
	return dst, nil
}

func (pf PreparedFrame) encodedSize() (int, error) {
	if pf.state == nil {
		return 0, errors.New("jobmgr frame owner: unprepared frame")
	}
	pf.state.mu.Lock()
	defer pf.state.mu.Unlock()
	if pf.state.consumed {
		return 0, ErrPreparedFrameConsumed
	}
	return pf.state.encodedBytes, nil
}

func (pf PreparedFrame) take() (preparedFunctionFrameContent, error) {
	if pf.state == nil {
		return preparedFunctionFrameContent{}, errors.New("jobmgr frame owner: unprepared frame")
	}
	pf.state.mu.Lock()
	defer pf.state.mu.Unlock()
	if pf.state.consumed {
		return preparedFunctionFrameContent{}, ErrPreparedFrameConsumed
	}
	pf.state.consumed = true
	content := preparedFunctionFrameContent{
		uid: pf.state.uid, result: pf.state.result, expiry: pf.state.expiry, encodedBytes: pf.state.encodedBytes,
	}
	pf.state.result = SealedResult{}
	return content, nil
}

func PrepareProtocolFrame(payload []byte) (PreparedProtocolFrame, error) {
	if len(payload) == 0 || len(payload) > MaximumOtherFrameBytes {
		return PreparedProtocolFrame{}, errors.New("jobmgr frame owner: invalid protocol frame size")
	}
	return PreparedProtocolFrame{state: &preparedProtocolFrame{payload: slices.Clone(payload)}}, nil
}

func (ppf PreparedProtocolFrame) Abort() error {
	_, err := ppf.take()
	return err
}

func (ppf PreparedProtocolFrame) take() ([]byte, error) {
	if ppf.state == nil {
		return nil, errors.New("jobmgr frame owner: unprepared protocol frame")
	}
	ppf.state.mu.Lock()
	defer ppf.state.mu.Unlock()
	if ppf.state.consumed {
		return nil, ErrPreparedFrameConsumed
	}
	ppf.state.consumed = true
	payload := ppf.state.payload
	ppf.state.payload = nil
	return payload, nil
}

func (fo *FrameOwner) CommitProtocolFrame(payload []byte) error {
	frame, err := PrepareProtocolFrame(payload)
	if err != nil {
		return err
	}
	return fo.CommitPreparedProtocolFrame(frame)
}

// CommitBorrowedProtocolFrame commits a complete caller-owned frame without a
// success-path copy. A failed write retains its own copy before returning.
func (fo *FrameOwner) CommitBorrowedProtocolFrame(payload []byte) error {
	if len(payload) == 0 || len(payload) > MaximumOtherFrameBytes {
		return errors.New("jobmgr frame owner: invalid borrowed protocol frame size")
	}
	return fo.commitOrdinaryTransaction(payload, nil, true)
}

// CommitBorrowedProtocolTransaction publishes bytes before making their
// corresponding in-memory state visible. A failed write runs abort instead.
func (fo *FrameOwner) CommitBorrowedProtocolTransaction(
	payload []byte,
	transaction ProtocolTransaction,
) error {
	if transaction == nil {
		return errors.New("jobmgr frame owner: invalid borrowed protocol transaction")
	}
	if len(payload) == 0 || len(payload) > MaximumOtherFrameBytes {
		return errors.Join(
			errors.New("jobmgr frame owner: invalid borrowed protocol transaction"),
			callFrameTransition("abort", transaction, false),
		)
	}
	return fo.commitOrdinaryTransaction(payload, transaction, true)
}

func (fo *FrameOwner) CommitPreparedProtocolFrame(frame PreparedProtocolFrame) error {
	payload, err := frame.take()
	if err != nil {
		return err
	}
	return fo.commitOrdinary(payload)
}

func (fo *FrameOwner) commitOrdinary(payload []byte) error {
	return fo.commitOrdinaryTransaction(payload, nil, false)
}

func (fo *FrameOwner) commitOrdinaryTransaction(
	payload []byte,
	transaction ProtocolTransaction,
	borrowed bool,
) error {
	fo.stateMu.Lock()
	for (fo.busy || fo.pendingControl) && !fo.poisoned {
		fo.available.Wait()
	}
	if fo.poisoned {
		fo.stateMu.Unlock()
		return errors.Join(
			ErrFrameOwnerPoisoned,
			callFrameTransition("abort", transaction, false),
		)
	}
	fo.busy = true
	fo.stateMu.Unlock()
	return fo.writeAndRelease(payload, borrowed, transaction)
}

func (fo *FrameOwner) TryCommitControl(plan ControlFramePlan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	fo.stateMu.Lock()
	if fo.poisoned {
		fo.stateMu.Unlock()
		return ErrFrameOwnerPoisoned
	}
	fo.pendingControl = true
	if fo.busy {
		fo.stateMu.Unlock()
		return ErrFrameOwnerBusy
	}
	fo.busy = true
	fo.pendingControl = false
	fo.stateMu.Unlock()

	payload := controlPayload(plan.Status)
	encoded, err := encodeResult(fo.controlBuffer[:0], plan.UID, int(plan.Status), "application/json", plan.Expiry, payload, ControlFrameBytes, 0, 0)
	if err != nil {
		fo.poison(encoded, err)
		return err
	}
	return fo.writeAndRelease(encoded, false, nil)
}

func (fo *FrameOwner) Census() FrameCensus {
	fo.stateMu.Lock()
	defer fo.stateMu.Unlock()
	return FrameCensus{Poisoned: fo.poisoned, Busy: fo.busy, PendingControl: fo.pendingControl, RetainedBytes: len(fo.retained)}
}

func (fo *FrameOwner) Poison(cause error) {
	if fo == nil {
		return
	}
	fo.poison(nil, cause)
}

func (fo *FrameOwner) writeAndRelease(
	payload []byte,
	borrowed bool,
	transaction ProtocolTransaction,
) error {
	count, err := fo.writer.Write(payload)
	if err != nil || count != len(payload) {
		if err == nil {
			err = io.ErrShortWrite
		}
		resultErr := errors.Join(
			fmt.Errorf("jobmgr frame owner: commit: %w", err),
			callFrameTransition("abort", transaction, false),
		)
		fo.poison(retainedFramePayload(payload, borrowed), resultErr)
		return resultErr
	}
	if err := callFrameTransition("commit", transaction, true); err != nil {
		err = errors.Join(
			err,
			callFrameTransition("abort", transaction, false),
		)
		fo.poison(retainedFramePayload(payload, borrowed), err)
		return err
	}
	fo.stateMu.Lock()
	fo.busy = false
	pending := fo.pendingControl
	notify := fo.onControlReady
	observer := fo.runtimeObserver
	fo.available.Broadcast()
	fo.stateMu.Unlock()
	if observer != nil {
		observer.AddRuntimeCounter(RuntimeCounterFramesCommitted, 1)
	}
	if pending && notify != nil {
		notify()
	}
	return nil
}

func retainedFramePayload(payload []byte, borrowed bool) []byte {
	if borrowed {
		return slices.Clone(payload)
	}
	return payload
}

func callFrameTransition(
	name string,
	transaction ProtocolTransaction,
	commit bool,
) (err error) {
	if transaction == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w in frame protocol %s: %v",
				ErrTaskPanic,
				name,
				recovered,
			)
		}
	}()
	if commit {
		return transaction.Commit()
	}
	return transaction.Abort()
}

func (fo *FrameOwner) poison(payload []byte, cause error) {
	fo.stateMu.Lock()
	notify := fo.onPoisoned
	observer := fo.runtimeObserver
	first := !fo.poisoned
	fo.poisoned = true
	fo.busy = false
	if first {
		fo.poisonErr = errors.Join(ErrFrameOwnerPoisoned, cause)
		fo.retained = payload
	}
	poisonErr := fo.poisonErr
	fo.available.Broadcast()
	fo.stateMu.Unlock()
	if first && observer != nil {
		observer.AddRuntimeCounter(RuntimeCounterFrameFailures, 1)
	}
	if first && notify != nil {
		notify(poisonErr)
	}
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
	return checkedSum(int(^uint(0)>>1), values...)
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
