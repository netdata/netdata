// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence(t *testing.T) {
	reader, writer := io.Pipe()
	ingress, err := NewProcessIngress(reader, lifecycle.NewAdmissionLedger())
	if err != nil {
		t.Fatal(err)
	}
	first := newTestProcessInput(1)
	second := newTestProcessInput(2)
	if err := ingress.Adopt(context.Background(), ProcessBinding{port: first, admission: ingress.admission}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()

	writeFunctionLine(t, writer, "first")
	firstCall := <-first.calls
	if firstCall.UID != "first" {
		t.Fatalf("first call differs: %+v", firstCall)
	}
	if err := ingress.SealPause(); err != nil {
		t.Fatal(err)
	}
	paused := make(chan error, 1)
	go func() { paused <- ingress.DrainPause(context.Background(), 2) }()
	select {
	case err := <-paused:
		t.Fatalf("pause bypassed an acquired delivery: %v", err)
	default:
	}
	close(first.release)
	if err := <-paused; err != nil {
		t.Fatal(err)
	}

	writeFunctionLine(t, writer, "carried")
	select {
	case call := <-first.calls:
		t.Fatalf("paused call reached old run: %+v", call)
	case call := <-second.calls:
		t.Fatalf("paused call reached successor before adopt: %+v", call)
	default:
	}
	if err := ingress.Adopt(context.Background(), ProcessBinding{port: second, admission: ingress.admission}); err != nil {
		t.Fatal(err)
	}
	carriedCall := <-second.calls
	if carriedCall.UID != "carried" {
		t.Fatalf("carried call differs: %+v", carriedCall)
	}
	close(second.release)
	writeFunctionLine(t, writer, "second")
	secondCall := <-second.calls
	if secondCall.UID != "second" {
		t.Fatalf("second call differs: %+v", secondCall)
	}
	if err := ingress.Pause(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := ingress.Fence(context.Background()); err != nil {
		t.Fatal(err)
	}
	ingress.boundary.mu.Lock()
	if ingress.boundary.target != nil ||
		!ingress.boundary.contained ||
		ingress.boundary.state != ProcessIngressContained {
		t.Fatalf("final capsule boundary retained a generation capability: %+v", ingress.boundary)
	}
	ingress.boundary.mu.Unlock()
	writeFunctionLine(t, writer, "contained")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if census := ingress.Census(); census.State != ProcessIngressContained ||
		census.ReaderStarts != 1 ||
		census.RunGeneration != 0 ||
		census.ActiveDeliveries != 0 ||
		census.PendingBody {
		t.Fatalf("final process ingress census differs: %+v", census)
	}
	if err := ingress.Run(context.Background()); err == nil {
		t.Fatal("process input reader started twice")
	}
}

func TestProcessIngressTimedOutPauseCanBeRetriedWithoutSplitState(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ingress, err := NewProcessIngress(reader, lifecycle.NewAdmissionLedger())
	if err != nil {
		t.Fatal(err)
	}
	input := newTestProcessInput(1)
	if err := ingress.Adopt(context.Background(), ProcessBinding{port: input, admission: ingress.admission}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()
	writeFunctionLine(t, writer, "held")
	<-input.calls
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := ingress.Pause(ctx, 0); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed pause result differs: %v", err)
	}
	ingress.boundary.mu.Lock()
	boundaryState := ingress.boundary.state
	ingress.boundary.mu.Unlock()
	if census := ingress.Census(); census.State != ProcessIngressLive || boundaryState != ProcessIngressLive {
		t.Fatalf("timed pause left split state: ingress=%+v boundary=%s", census, boundaryState)
	}
	close(input.release)
	if err := ingress.Pause(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := ingress.Fence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func writeFunctionLine(t *testing.T, writer io.Writer, uid string) {
	t.Helper()
	line := "FUNCTION " + uid + " 30 \"test:work\" 0xFFFF \"method=api,role=test\"\n"
	if _, err := io.WriteString(writer, line); err != nil {
		t.Fatal(err)
	}
}

type testProcessInput struct {
	generation uint64
	calls      chan functionwire.Call
	release    chan struct{}
}

func newTestProcessInput(generation uint64) *testProcessInput {
	return &testProcessInput{
		generation: generation,
		calls:      make(chan functionwire.Call, 2),
		release:    make(chan struct{}),
	}
}

func (input *testProcessInput) Generation() uint64 { return input.generation }

func (*testProcessInput) SuspendInputBody(uint64, uint64) error { return nil }
func (*testProcessInput) AdoptInputBody(uint64) error           { return nil }

func (input *testProcessInput) HandleCall(_ context.Context, call functionwire.Call) error {
	input.calls <- call
	<-input.release
	return nil
}

func (*testProcessInput) HandleCancel(context.Context, string) error      { return nil }
func (*testProcessInput) HandleReject(context.Context, string, int) error { return nil }
func (*testProcessInput) HandleQuit(context.Context) error                { return nil }

func (*testProcessInput) GrowInputBody(context.Context, uint64, int64) (uint64, error) {
	return 1, nil
}

func (*testProcessInput) CommitInputBodyGrowth(uint64, int64) error { return nil }
func (*testProcessInput) ReleaseInputBody(uint64) error             { return nil }
