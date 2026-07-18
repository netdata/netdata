package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

const SchemaVersion = 1

type Action string

const (
	ActionHoldDependency    Action = "hold_dependency"
	ActionReleaseDependency Action = "release_dependency"
	ActionSetWriteFault     Action = "set_write_fault"
	ActionSetChildBehavior  Action = "set_child_behavior"
	ActionAdvanceClock      Action = "advance_clock"
)

type EventKind string

const (
	EventCutReached       EventKind = "cut-reached"
	EventHandlerEntered   EventKind = "handler-entered"
	EventHandlerReturned  EventKind = "handler-returned"
	EventDeadlineObserved EventKind = "deadline-observed"
	EventWriteAttempt     EventKind = "write-attempt"
	EventProcessSentinel  EventKind = "process-sentinel"
)

type ErrorClass string

const (
	ErrorNone              ErrorClass = ""
	ErrorInvalidFrame      ErrorClass = "invalid-frame"
	ErrorInvalidGeneration ErrorClass = "invalid-generation"
	ErrorInvalidSequence   ErrorClass = "invalid-sequence"
	ErrorInvalidAction     ErrorClass = "invalid-action"
	ErrorInvalidPayload    ErrorClass = "invalid-payload"
	ErrorUnsupportedCut    ErrorClass = "unsupported-cut"
	ErrorInternal          ErrorClass = "internal"
)

// Control carries transport actions only. Evaluation identity and expected
// outcomes belong exclusively to the neutral parent.
type Control struct {
	SchemaVersion     uint32 `json:"schema_version"`
	ProcessGeneration uint64 `json:"process_generation"`
	ControlSeq        uint64 `json:"control_seq"`
	Action            Action `json:"action"`
	CutID             string `json:"cut_id"`
	PayloadLen        uint32 `json:"payload_len"`
	PayloadSHA256     string `json:"payload_sha256"`
	Payload           []byte `json:"payload"`
}

// Ack acknowledges transport acceptance without claiming lifecycle state.
type Ack struct {
	SchemaVersion     uint32     `json:"schema_version"`
	ProcessGeneration uint64     `json:"process_generation"`
	ControlSeq        uint64     `json:"control_seq"`
	CutID             string     `json:"cut_id"`
	Accepted          bool       `json:"accepted"`
	ErrorClass        ErrorClass `json:"error_class"`
}

// Event reports a passive boundary fact. Parent-owned reduction assigns any
// lifecycle meaning after the event crosses the process boundary.
type Event struct {
	SchemaVersion     uint32    `json:"schema_version"`
	ProcessGeneration uint64    `json:"process_generation"`
	EventSeq          uint64    `json:"event_seq"`
	Kind              EventKind `json:"event"`
	Token             string    `json:"token"`
	RouteKey          string    `json:"route_key"`
	PayloadLen        uint32    `json:"payload_len"`
	PayloadSHA256     string    `json:"payload_sha256"`
	Payload           []byte    `json:"payload"`
}

type FactKind uint8

const (
	FactAck FactKind = iota + 1
	FactEvent
)

type Fact struct {
	Kind  FactKind
	Ack   Ack
	Event Event
}

var opaqueIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)

func NewControl(generation, sequence uint64, action Action, cutID string, payload []byte) (Control, error) {
	payload = append([]byte{}, payload...)
	message := Control{
		SchemaVersion: SchemaVersion, ProcessGeneration: generation, ControlSeq: sequence,
		Action: action, CutID: cutID, PayloadLen: uint32(len(payload)), PayloadSHA256: digest(payload), Payload: payload,
	}
	return message, message.Validate()
}

func NewEvent(generation, sequence uint64, kind EventKind, token, routeKey string, payload []byte) (Event, error) {
	payload = append([]byte{}, payload...)
	message := Event{
		SchemaVersion: SchemaVersion, ProcessGeneration: generation, EventSeq: sequence,
		Kind: kind, Token: token, RouteKey: routeKey, PayloadLen: uint32(len(payload)), PayloadSHA256: digest(payload), Payload: payload,
	}
	return message, message.Validate()
}

func (message Control) Validate() error {
	if err := validateHeader(message.SchemaVersion, message.ProcessGeneration, message.ControlSeq); err != nil {
		return err
	}
	if !validAction(message.Action) {
		return fmt.Errorf("evaluator protocol: invalid action %q", message.Action)
	}
	if !opaqueIDPattern.MatchString(message.CutID) {
		return errors.New("evaluator protocol: invalid cut_id")
	}
	return validatePayload(message.Payload, message.PayloadLen, message.PayloadSHA256)
}

func (message Ack) Validate() error {
	if err := validateHeader(message.SchemaVersion, message.ProcessGeneration, message.ControlSeq); err != nil {
		return err
	}
	if !opaqueIDPattern.MatchString(message.CutID) {
		return errors.New("evaluator protocol: invalid cut_id")
	}
	if !validErrorClass(message.ErrorClass) {
		return fmt.Errorf("evaluator protocol: invalid error_class %q", message.ErrorClass)
	}
	if message.Accepted != (message.ErrorClass == ErrorNone) {
		return errors.New("evaluator protocol: accepted/error_class mismatch")
	}
	return nil
}

func (message Event) Validate() error {
	if err := validateHeader(message.SchemaVersion, message.ProcessGeneration, message.EventSeq); err != nil {
		return err
	}
	if !validEventKind(message.Kind) {
		return fmt.Errorf("evaluator protocol: invalid event %q", message.Kind)
	}
	if message.Token != "" && !opaqueIDPattern.MatchString(message.Token) {
		return errors.New("evaluator protocol: invalid token")
	}
	if message.RouteKey != "" && !opaqueIDPattern.MatchString(message.RouteKey) {
		return errors.New("evaluator protocol: invalid route_key")
	}
	return validatePayload(message.Payload, message.PayloadLen, message.PayloadSHA256)
}

func WriteControl(w io.Writer, message Control) error { return writeMessage(w, message) }
func WriteAck(w io.Writer, message Ack) error         { return writeMessage(w, message) }
func WriteEvent(w io.Writer, message Event) error     { return writeMessage(w, message) }

func ReadControl(r io.Reader) (Control, error) { return readMessage[Control](r) }
func ReadAck(r io.Reader) (Ack, error)         { return readMessage[Ack](r) }
func ReadEvent(r io.Reader) (Event, error)     { return readMessage[Event](r) }

func ReadFact(r io.Reader) (Fact, error) {
	payload, err := ReadFrame(r)
	if err != nil {
		return Fact{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Fact{}, fmt.Errorf("decode evaluator protocol fact: %w", err)
	}
	if _, ok := fields["event"]; ok {
		message, err := decodeMessage[Event](payload)
		return Fact{Kind: FactEvent, Event: message}, err
	}
	if _, ok := fields["accepted"]; ok {
		message, err := decodeMessage[Ack](payload)
		return Fact{Kind: FactAck, Ack: message}, err
	}
	return Fact{}, errors.New("decode evaluator protocol fact: unknown kind")
}

func writeMessage[T interface{ Validate() error }](w io.Writer, message T) error {
	if err := message.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal evaluator protocol: %w", err)
	}
	return WriteFrame(w, payload)
}

func readMessage[T interface{ Validate() error }](r io.Reader) (T, error) {
	payload, err := ReadFrame(r)
	if err != nil {
		var message T
		return message, err
	}
	return decodeMessage[T](payload)
}

func decodeMessage[T interface{ Validate() error }](payload []byte) (T, error) {
	var message T
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return message, fmt.Errorf("decode evaluator protocol: %w", err)
	}
	if decoder.More() {
		return message, errors.New("decode evaluator protocol: trailing JSON value")
	}
	canonical, err := json.Marshal(message)
	if err != nil {
		return message, fmt.Errorf("remarshal evaluator protocol: %w", err)
	}
	if !bytes.Equal(payload, canonical) {
		return message, errors.New("decode evaluator protocol: non-canonical JSON")
	}
	if err := message.Validate(); err != nil {
		return message, err
	}
	return message, nil
}

func validateHeader(version uint32, generation, sequence uint64) error {
	if version != SchemaVersion {
		return fmt.Errorf("evaluator protocol: schema version %d, want %d", version, SchemaVersion)
	}
	if generation == 0 {
		return errors.New("evaluator protocol: process generation must be positive")
	}
	if sequence == 0 {
		return errors.New("evaluator protocol: sequence must be positive")
	}
	return nil
}

func validatePayload(payload []byte, declaredLen uint32, declaredDigest string) error {
	if uint64(len(payload)) > uint64(MaxFrameBytes) {
		return ErrFrameTooLarge
	}
	if uint32(len(payload)) != declaredLen {
		return errors.New("evaluator protocol: payload length mismatch")
	}
	if declaredDigest != digest(payload) {
		return errors.New("evaluator protocol: payload digest mismatch")
	}
	return nil
}

func validAction(action Action) bool {
	switch action {
	case ActionHoldDependency, ActionReleaseDependency, ActionSetWriteFault, ActionSetChildBehavior, ActionAdvanceClock:
		return true
	default:
		return false
	}
}

func validEventKind(kind EventKind) bool {
	switch kind {
	case EventCutReached, EventHandlerEntered, EventHandlerReturned, EventDeadlineObserved, EventWriteAttempt, EventProcessSentinel:
		return true
	default:
		return false
	}
}

func validErrorClass(class ErrorClass) bool {
	switch class {
	case ErrorNone, ErrorInvalidFrame, ErrorInvalidGeneration, ErrorInvalidSequence, ErrorInvalidAction, ErrorInvalidPayload, ErrorUnsupportedCut, ErrorInternal:
		return true
	default:
		return false
	}
}

func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
