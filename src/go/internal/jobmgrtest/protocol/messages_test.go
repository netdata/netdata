package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestControlRoundTrip(t *testing.T) {
	want, err := NewControl(3, 7, ActionHoldDependency, "dependency-1", []byte("opaque"))
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	if err := WriteControl(&wire, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadControl(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip differs:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFactDecoderDistinguishesAcknowledgementsAndEvents(t *testing.T) {
	control, err := NewControl(3, 7, ActionHoldDependency, "handler.before", nil)
	if err != nil {
		t.Fatal(err)
	}
	ack := Ack{SchemaVersion: SchemaVersion, ProcessGeneration: 3, ControlSeq: 7, CutID: control.CutID, Accepted: true}
	event, err := NewEvent(3, 8, EventCutReached, "u1", "handler.before", nil)
	if err != nil {
		t.Fatal(err)
	}
	var wire bytes.Buffer
	if err := WriteAck(&wire, ack); err != nil {
		t.Fatal(err)
	}
	if err := WriteEvent(&wire, event); err != nil {
		t.Fatal(err)
	}
	first, err := ReadFact(&wire)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ReadFact(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != FactAck || first.Ack != ack || second.Kind != FactEvent || second.Event.Kind != event.Kind || second.Event.Token != event.Token {
		t.Fatalf("facts differ: first=%#v second=%#v", first, second)
	}
}

func TestProtocolDTOsExposeNoEvaluationIdentity(t *testing.T) {
	forbidden := map[string]struct{}{
		"case_id": {}, "predicate_id": {}, "implementation_variant": {}, "oracle_profile_ref": {},
		"parity_group_id": {}, "parity_anchor_case_id": {}, "coverage_row_ids": {}, "expected": {},
		"expected_outcome": {}, "mutation_role": {}, "authority_class": {}, "verdict": {}, "pass": {}, "fail": {},
		"workload": {}, "population": {}, "threshold": {},
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[Control](), reflect.TypeFor[Ack](), reflect.TypeFor[Event]()} {
		for index := range typ.NumField() {
			name := strings.Split(typ.Field(index).Tag.Get("json"), ",")[0]
			if _, exists := forbidden[name]; exists {
				t.Fatalf("%s exposes evaluator-owned identity field %q", typ.Name(), name)
			}
		}
	}
}

func TestReadControlRejectsUnknownAndNoncanonicalJSON(t *testing.T) {
	message, err := NewControl(1, 1, ActionReleaseDependency, "dependency-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(canonical, []byte(`"control_seq":1`), []byte(`"case_id":"F01.1","control_seq":1`), 1)
	if _, err := ReadControl(bytes.NewReader(frame(t, unknown))); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown evaluation identity was not rejected: %v", err)
	}
	spaced := append([]byte(" "), canonical...)
	if _, err := ReadControl(bytes.NewReader(frame(t, spaced))); err == nil || !strings.Contains(err.Error(), "non-canonical") {
		t.Fatalf("noncanonical JSON was not rejected: %v", err)
	}
}

func TestControlRejectsPayloadMismatch(t *testing.T) {
	message, err := NewControl(1, 1, ActionSetChildBehavior, "child-1", []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	message.Payload = []byte("b")
	if err := message.Validate(); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("payload mutation was not rejected: %v", err)
	}
}

func TestFrameBoundsAndShortWrites(t *testing.T) {
	for _, size := range []int{0, MaxFrameBytes + 1} {
		err := WriteFrame(io.Discard, make([]byte, size))
		if size == 0 && !errors.Is(err, ErrEmptyFrame) {
			t.Fatalf("size 0: got %v", err)
		}
		if size > MaxFrameBytes && !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("oversize: got %v", err)
		}
	}
	writer := &shortWriter{limit: 3}
	if err := WriteFrame(writer, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if got := writer.buffer.Bytes(); !bytes.Equal(got, frame(t, []byte("payload"))) {
		t.Fatalf("short-write recovery differs: %x", got)
	}
}

func frame(t *testing.T, payload []byte) []byte {
	t.Helper()
	var wire bytes.Buffer
	if err := binary.Write(&wire, binary.BigEndian, uint32(len(payload))); err != nil {
		t.Fatal(err)
	}
	wire.Write(payload)
	return wire.Bytes()
}

type shortWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (writer *shortWriter) Write(payload []byte) (int, error) {
	if len(payload) > writer.limit {
		payload = payload[:writer.limit]
	}
	return writer.buffer.Write(payload)
}
