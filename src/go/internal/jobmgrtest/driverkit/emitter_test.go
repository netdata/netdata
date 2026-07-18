package driverkit

import (
	"bytes"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
)

func TestEmitterSerializesConcurrentPassiveEvents(t *testing.T) {
	var wire bytes.Buffer
	emitter, err := NewEmitter(&wire, 7)
	if err != nil {
		t.Fatal(err)
	}
	const count = 32
	var wait sync.WaitGroup
	for range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := emitter.Event(protocol.EventWriteAttempt, "u00000000000000000000000000000000", "writer", nil); err != nil {
				t.Errorf("emit: %v", err)
			}
		}()
	}
	wait.Wait()
	for sequence := uint64(1); sequence <= count; sequence++ {
		event, err := protocol.ReadEvent(&wire)
		if err != nil {
			t.Fatal(err)
		}
		if event.EventSeq != sequence || event.ProcessGeneration != 7 {
			t.Fatalf("event %d differs: %#v", sequence, event)
		}
	}
}

func TestEmitterRejectsAckForAnotherGeneration(t *testing.T) {
	emitter, err := NewEmitter(&bytes.Buffer{}, 7)
	if err != nil {
		t.Fatal(err)
	}
	control, err := protocol.NewControl(8, 1, protocol.ActionReleaseDependency, "dependency-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := emitter.Ack(control, true, protocol.ErrorNone); err == nil {
		t.Fatal("cross-generation acknowledgement accepted")
	}
}
