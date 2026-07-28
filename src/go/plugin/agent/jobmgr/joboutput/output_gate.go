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
)

type generationOutputState uint8

const (
	generationOutputInactive generationOutputState = iota + 1
	generationOutputActive
	generationOutputFenced
)

// generationOutputGate linearizes a collector generation's output with its
// installation and retirement without changing the process-global writer.
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
