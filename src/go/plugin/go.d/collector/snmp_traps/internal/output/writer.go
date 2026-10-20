// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

var (
	ErrQueueFull  = errors.New("trap writer queue is full")
	ErrClosed     = errors.New("trap writer is closed")
	ErrNotStarted = errors.New("trap writer is not started")
)

// Writer accepts an immutable entry. A coordinator may retain the entry through
// another backend even when it returns an authority error, so callers must not
// mutate or reuse the entry or reachable backing state after Write returns.
// Concurrent immutable reads remain valid. An individual backend retains the
// entry only when its own Write succeeds.
type Writer interface {
	Write(entry *model.TrapEntry) error
	Flush() error
	Close() error
}

type BinaryFieldCounter interface {
	BinaryEncodedFields() uint64
}

type Backend uint8

const (
	BackendJournal Backend = iota + 1
	BackendOTLP
)

type Stage uint8

const (
	StageEnqueue Stage = iota + 1
	StageExport
	StageWorker
)

type Outcome struct {
	Backend       Backend
	Stage         Stage
	FailedEntries uint64
	Err           error
	Authoritative bool
}

type OutcomeReporter func(Outcome)

func (report OutcomeReporter) Report(outcome Outcome) {
	if report != nil {
		report(outcome)
	}
}
