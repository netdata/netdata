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

func (gog *generationOutputGate) Activate() error {
	if gog == nil {
		return errors.New("job output: nil generation output gate")
	}
	gog.mu.Lock()
	defer gog.mu.Unlock()
	if gog.state != generationOutputInactive {
		return errors.New("job output: invalid generation output activation")
	}
	gog.state = generationOutputActive
	return nil
}

func (gog *generationOutputGate) Fence() {
	if gog == nil {
		return
	}
	gog.mu.Lock()
	gog.state = generationOutputFenced
	gog.mu.Unlock()
}

func (gog *generationOutputGate) Write(payload []byte) (int, error) {
	if gog == nil {
		return 0, errGenerationOutputInactive
	}
	gog.mu.RLock()
	defer gog.mu.RUnlock()
	switch gog.state {
	case generationOutputActive:
		return gog.writer.Write(payload)
	case generationOutputFenced:
		return 0, errGenerationOutputFenced
	default:
		return 0, errGenerationOutputInactive
	}
}

func (gog *generationOutputGate) CommitJobOutput(
	payload []byte,
	transaction jobruntime.OutputStateTransaction,
) error {
	if transaction == nil {
		return errors.New("job output: invalid generation output transaction")
	}
	if gog == nil {
		return errors.Join(errGenerationOutputInactive, transaction.Abort())
	}
	gog.mu.RLock()
	defer gog.mu.RUnlock()
	switch gog.state {
	case generationOutputActive:
		return gog.writer.CommitJobOutput(payload, transaction)
	case generationOutputFenced:
		return errors.Join(errGenerationOutputFenced, transaction.Abort())
	default:
		return errors.Join(errGenerationOutputInactive, transaction.Abort())
	}
}

func (gog *generationOutputGate) PoisonOutput(err error) {
	if gog == nil ||
		errors.Is(err, errGenerationOutputInactive) ||
		errors.Is(err, errGenerationOutputFenced) {
		return
	}
	gog.writer.PoisonOutput(err)
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

func (cog *CleanupOutputGate) Fence() {
	if cog == nil {
		return
	}
	cog.mu.Lock()
	cog.fenced = true
	cog.mu.Unlock()
}

func (cog *CleanupOutputGate) Write(payload []byte) (int, error) {
	if cog == nil {
		return 0, errCleanupOutputFenced
	}
	cog.mu.RLock()
	defer cog.mu.RUnlock()
	if cog.fenced {
		return 0, errCleanupOutputFenced
	}
	return cog.writer.Write(payload)
}

func (cog *CleanupOutputGate) PoisonOutput(err error) {
	if cog == nil || errors.Is(err, errCleanupOutputFenced) {
		return
	}
	cog.writer.PoisonOutput(err)
}
