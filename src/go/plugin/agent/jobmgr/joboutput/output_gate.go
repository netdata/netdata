// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

var (
	errGenerationOutputInactive = errors.New("job output: generation output is inactive")
	errGenerationOutputFenced   = errors.New("job output: generation output is fenced")
	errCleanupOutputFenced      = errors.New("job output: cleanup output is fenced")
)

type generationOutputState uint8

const (
	generationOutputInactive generationOutputState = iota + 1
	generationOutputActive
	generationOutputFenced
)

// generationOutputGate linearizes a collector generation's output with its
// installation and retirement without changing the process-global writer.
// Its read lease deliberately spans frame I/O so Fence waits for every
// admitted write. A permanently blocked process stdout is not recoverable
// inside Job Manager.
type generationOutputGate struct {
	mu     sync.RWMutex
	writer FrameWriter
	state  generationOutputState
}

func newGenerationOutputGate(owner *lifecycle.FrameOwner) (*generationOutputGate, error) {
	if owner == nil {
		return nil, errors.New("job output: nil generation output owner")
	}
	return &generationOutputGate{
		writer: FrameWriter{Owner: owner},
		state:  generationOutputInactive,
	}, nil
}

func (gate *generationOutputGate) Activate() error {
	if gate == nil {
		return errors.New("job output: nil generation output gate")
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.state != generationOutputInactive {
		return errors.New("job output: invalid generation output activation")
	}
	gate.state = generationOutputActive
	return nil
}

func (gate *generationOutputGate) Fence() {
	if gate == nil {
		return
	}
	gate.mu.Lock()
	gate.state = generationOutputFenced
	gate.mu.Unlock()
}

func (gate *generationOutputGate) Write(payload []byte) (int, error) {
	if gate == nil {
		return 0, errGenerationOutputInactive
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	switch gate.state {
	case generationOutputActive:
		return gate.writer.Write(payload)
	case generationOutputFenced:
		return 0, errGenerationOutputFenced
	default:
		return 0, errGenerationOutputInactive
	}
}

func (gate *generationOutputGate) CommitJobOutput(
	payload []byte,
	transaction jobruntime.OutputStateTransaction,
) error {
	if transaction == nil {
		return errors.New("job output: invalid generation output transaction")
	}
	if gate == nil {
		return errors.Join(errGenerationOutputInactive, transaction.Abort())
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	switch gate.state {
	case generationOutputActive:
		return gate.writer.CommitJobOutput(payload, transaction)
	case generationOutputFenced:
		return errors.Join(errGenerationOutputFenced, transaction.Abort())
	default:
		return errors.Join(errGenerationOutputInactive, transaction.Abort())
	}
}

func (gate *generationOutputGate) PoisonOutput(err error) {
	if gate == nil ||
		errors.Is(err, errGenerationOutputInactive) ||
		errors.Is(err, errGenerationOutputFenced) {
		return
	}
	gate.writer.PoisonOutput(err)
}

// CleanupOutputGate is the process-lifetime output capability used only by
// accepted collector cleanup. Fence terminally rejects cleanup reached after
// bounded process shutdown.
type CleanupOutputGate struct {
	mu     sync.RWMutex
	writer FrameWriter
	fenced bool
}

// NewCleanupOutputGate binds accepted cleanup to the process frame owner.
func NewCleanupOutputGate(owner *lifecycle.FrameOwner) (*CleanupOutputGate, error) {
	if owner == nil {
		return nil, errors.New("job output: nil cleanup output owner")
	}
	return &CleanupOutputGate{
		writer: FrameWriter{Owner: owner},
	}, nil
}

func (gate *CleanupOutputGate) Fence() {
	if gate == nil {
		return
	}
	gate.mu.Lock()
	gate.fenced = true
	gate.mu.Unlock()
}

func (gate *CleanupOutputGate) Write(payload []byte) (int, error) {
	if gate == nil {
		return 0, errCleanupOutputFenced
	}
	gate.mu.RLock()
	defer gate.mu.RUnlock()
	if gate.fenced {
		return 0, errCleanupOutputFenced
	}
	return gate.writer.Write(payload)
}

func (gate *CleanupOutputGate) PoisonOutput(err error) {
	if gate == nil || errors.Is(err, errCleanupOutputFenced) {
		return
	}
	gate.writer.PoisonOutput(err)
}
