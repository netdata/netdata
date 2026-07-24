// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestInputCapsuleParsesFunctionCancelAndQuit(t *testing.T) {
	consumer := &recordingCapsuleConsumer{}
	capsule, err := NewInputCapsule(strings.NewReader(
		"FUNCTION u1 30 \"perf:work-001 mode:F token:u1\" 0xFFFF \"method=api,role=test\"\n" +
			"FUNCTION_CANCEL u1\nQUIT\n" +
			"FUNCTION u2 30 \"perf:work-002 mode:F token:u2\" 0xFFFF \"method=api,role=test\"\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := capsule.Run(context.Background(), consumer); err != nil {
		t.Fatal(err)
	}
	if len(consumer.calls) != 1 ||
		consumer.calls[0].Method != "perf:work-001" ||
		len(consumer.calls[0].Args) != 2 ||
		consumer.cancel != "u1" ||
		!consumer.quit {
		t.Fatalf("parsed input differs: %#v", consumer)
	}
}

func TestInputCapsuleResynchronizesSafeUIDHeaderErrors(t *testing.T) {
	consumer := &recordingCapsuleConsumer{}
	oversized := "FUNCTION oversized 30 \"" + strings.Repeat("x", maximumInputLineBytes) + "\" 0xFFFF \"source\"\n"
	input := "FUNCTION bad-time nope \"x\" 0xFFFF \"source\"\n" + oversized +
		"FUNCTION good 30 \"perf:work-001 mode:F token:good\" 0xFFFF \"source\"\nQUIT\n"
	capsule, err := NewInputCapsule(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := capsule.Run(context.Background(), consumer); err != nil {
		t.Fatal(err)
	}
	if len(consumer.rejections) != 2 ||
		consumer.rejections[0] != (recordedCapsuleRejection{uid: "bad-time", status: 400}) ||
		consumer.rejections[1] != (recordedCapsuleRejection{uid: "oversized", status: 400}) {
		t.Fatalf("recoverable rejections differ: %#v", consumer.rejections)
	}
	if len(consumer.calls) != 1 || consumer.calls[0].UID != "good" || !consumer.quit {
		t.Fatalf("post-rejection flow differs: %#v", consumer)
	}
}

func TestInputCapsuleRejectsUnaddressableOrUnterminatedInput(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"cancel without UID": {input: "FUNCTION_CANCEL\n"},
		"unknown command":    {input: "GARBAGE\n"},
		"unterminated quit":  {input: "QUIT"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			capsule, err := NewInputCapsule(strings.NewReader(test.input))
			if err != nil {
				t.Fatal(err)
			}
			if err := capsule.Run(context.Background(), &recordingCapsuleConsumer{}); err == nil {
				t.Fatalf("malformed input accepted: %q", test.input)
			}
		})
	}
}

func TestInputCapsuleCommandLineAndTimeoutBoundaries(t *testing.T) {
	exactLine := capsuleFunctionLineOfLength(t, "line-exact", 30, MaximumCommandLineBytes)
	overLine := capsuleFunctionLineOfLength(t, "line-over", 30, MaximumCommandLineBytes+1)
	input := exactLine + "\n" + overLine + "\n" +
		"FUNCTION timeout-negative -1 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-overflow 9223372036854775808 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-zero 0 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-one 1 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-at-max 120 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-over 121 \"perf:work\" 0xFFFF \"source\"\n" +
		"FUNCTION timeout-millis 60000 \"perf:work\" 0xFFFF \"source\"\nQUIT\n"
	consumer := &recordingCapsuleConsumer{}
	capsule, err := NewInputCapsule(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := capsule.Run(context.Background(), consumer); err != nil {
		t.Fatal(err)
	}
	// Unset (0) and any value above the maximum clamp to MaximumFunctionTimeout;
	// 1..max pass through unchanged.
	wantAccepted := []struct {
		uid     string
		timeout time.Duration
	}{
		{uid: "line-exact", timeout: 30 * time.Second},
		{uid: "timeout-zero", timeout: maximumFunctionTimeout},
		{uid: "timeout-one", timeout: time.Second},
		{uid: "timeout-at-max", timeout: maximumFunctionTimeout},
		{uid: "timeout-over", timeout: maximumFunctionTimeout},
		{uid: "timeout-millis", timeout: maximumFunctionTimeout},
	}
	if len(consumer.calls) != len(wantAccepted) {
		t.Fatalf("accepted count differs: got=%d want=%d calls=%#v", len(consumer.calls), len(wantAccepted), consumer.calls)
	}
	for i, want := range wantAccepted {
		if consumer.calls[i].UID != want.uid || consumer.calls[i].Timeout != want.timeout {
			t.Fatalf("accepted call %d differs: got uid=%q timeout=%v want uid=%q timeout=%v",
				i, consumer.calls[i].UID, consumer.calls[i].Timeout, want.uid, want.timeout)
		}
	}
	wantRejected := []recordedCapsuleRejection{
		{uid: "line-over", status: 400},
		{uid: "timeout-negative", status: 400},
		{uid: "timeout-overflow", status: 400},
	}
	if fmt.Sprint(consumer.rejections) != fmt.Sprint(wantRejected) || !consumer.quit {
		t.Fatalf("rejected command boundaries differ: got=%#v want=%#v quit=%v", consumer.rejections, wantRejected, consumer.quit)
	}
}

func capsuleFunctionLineOfLength(t *testing.T, uid string, timeout, length int) string {
	t.Helper()
	prefix := fmt.Sprintf("FUNCTION %s %d \"perf:work ", uid, timeout)
	suffix := "\" 0xFFFF \"source\""
	padding := length - len(prefix) - len(suffix)
	if padding < 0 {
		t.Fatalf("requested command length %d is too small", length)
	}
	line := prefix + strings.Repeat("x", padding) + suffix
	if len(line) != length {
		t.Fatalf("command length=%d, want %d", len(line), length)
	}
	return line
}

func TestInputCapsulePreservesPayloadBytesAndPreAdmissionCancel(t *testing.T) {
	consumer := &recordingCapsuleConsumer{}
	input := "FUNCTION_PAYLOAD raw 30 \"raw:echo\" 0xFFFF \"source\" application/octet-stream\r\n" +
		"  leading\tvalue  \r\n\r\ntail\nFUNCTION_PAYLOAD_END\r\n" +
		"FUNCTION_PAYLOAD cancelled 30 \"raw:echo\" 0xFFFF \"source\" application/octet-stream\n" +
		"partial\nFUNCTION_CANCEL cancelled\nQUIT\n"
	capsule, err := NewInputCapsule(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := capsule.Run(context.Background(), consumer); err != nil {
		t.Fatal(err)
	}
	if len(consumer.calls) != 1 {
		t.Fatalf("payload call count differs: %#v", consumer.calls)
	}
	call := consumer.calls[0]
	if !call.HasPayload ||
		call.ContentType != "application/octet-stream" ||
		string(call.Payload) != "  leading\tvalue  \r\n\r\ntail" {
		t.Fatalf("payload call differs: %#v payload=%q", call, call.Payload)
	}
	if len(consumer.rejections) != 1 ||
		consumer.rejections[0] != (recordedCapsuleRejection{uid: "cancelled", status: 499}) ||
		!consumer.quit {
		t.Fatalf("pre-admission cancel differs: %#v quit=%v", consumer.rejections, consumer.quit)
	}
}

func TestInputCapsulePayloadBoundariesAndResynchronization(t *testing.T) {
	tests := map[string]struct {
		size     int
		accepted bool
	}{
		"one byte below limit": {size: MaximumInputBodyBytes - 1, accepted: true},
		"at limit":             {size: MaximumInputBodyBytes, accepted: true},
		"one byte over limit":  {size: MaximumInputBodyBytes + 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			consumer := &recordingCapsuleConsumer{}
			input := "FUNCTION_PAYLOAD boundary 30 \"raw:echo\" 0xFFFF \"source\" application/octet-stream\n" +
				strings.Repeat("x", test.size) + "\nFUNCTION_PAYLOAD_END\n" +
				"FUNCTION successor 30 \"perf:work\" 0xFFFF \"source\"\nQUIT\n"
			capsule, err := NewInputCapsule(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			if err := capsule.Run(context.Background(), consumer); err != nil {
				t.Fatal(err)
			}
			if test.accepted {
				if len(consumer.calls) != 2 ||
					consumer.calls[0].UID != "boundary" ||
					len(consumer.calls[0].Payload) != test.size ||
					consumer.calls[1].UID != "successor" ||
					len(consumer.rejections) != 0 {
					t.Fatalf("accepted boundary flow differs: calls=%#v rejections=%#v", consumer.calls, consumer.rejections)
				}
			} else if len(consumer.calls) != 1 ||
				consumer.calls[0].UID != "successor" ||
				len(consumer.rejections) != 1 ||
				consumer.rejections[0] != (recordedCapsuleRejection{uid: "boundary", status: 413}) {
				t.Fatalf("overflow resynchronization differs: calls=%#v rejections=%#v", consumer.calls, consumer.rejections)
			}
		})
	}
}

type recordingCapsuleConsumer struct {
	calls      []Call
	rejections []recordedCapsuleRejection
	cancel     string
	quit       bool
}

type recordedCapsuleRejection struct {
	uid    string
	status int
}

func (consumer *recordingCapsuleConsumer) HandleCall(_ context.Context, call Call) error {
	consumer.calls = append(consumer.calls, call)
	return nil
}

func (consumer *recordingCapsuleConsumer) HandleCancel(_ context.Context, uid string) error {
	consumer.cancel = uid
	return nil
}

func (consumer *recordingCapsuleConsumer) HandleReject(_ context.Context, uid string, status int) error {
	consumer.rejections = append(consumer.rejections, recordedCapsuleRejection{uid: uid, status: status})
	return nil
}

func (consumer *recordingCapsuleConsumer) HandleQuit(context.Context) error {
	consumer.quit = true
	return nil
}
