package driverkit

import (
	"errors"
	"io"
	"sync"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/protocol"
)

// Emitter serializes passive child facts. It cannot assign cases, predicates,
// expected outcomes, or verdicts.
type Emitter struct {
	mu         sync.Mutex
	writer     io.Writer
	generation uint64
	sequence   uint64
}

func NewEmitter(writer io.Writer, generation uint64) (*Emitter, error) {
	if writer == nil {
		return nil, errors.New("evaluator driverkit: nil event writer")
	}
	if generation == 0 {
		return nil, errors.New("evaluator driverkit: generation must be positive")
	}
	return &Emitter{writer: writer, generation: generation}, nil
}

func (emitter *Emitter) Event(kind protocol.EventKind, token, routeKey string, payload []byte) error {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	emitter.sequence++
	message, err := protocol.NewEvent(emitter.generation, emitter.sequence, kind, token, routeKey, payload)
	if err != nil {
		return err
	}
	return protocol.WriteEvent(emitter.writer, message)
}

func (emitter *Emitter) Ack(control protocol.Control, accepted bool, class protocol.ErrorClass) error {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	if err := control.Validate(); err != nil {
		return err
	}
	if control.ProcessGeneration != emitter.generation {
		return errors.New("evaluator driverkit: control generation mismatch")
	}
	message := protocol.Ack{
		SchemaVersion: protocol.SchemaVersion, ProcessGeneration: emitter.generation,
		ControlSeq: control.ControlSeq, CutID: control.CutID, Accepted: accepted, ErrorClass: class,
	}
	return protocol.WriteAck(emitter.writer, message)
}
