// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Store encodes and queries synthetic runs in the command-owned journal.
// It borrows the journal; only the command may close it or enforce retention.
type Store struct{ journal *demjournal.Store }

func NewStore(journal *demjournal.Store) *Store {
	return &Store{
		journal: journal,
	}
}
func (s *Store) Sync(ctx context.Context) error { return s.journal.Sync(ctx) }

type RunFilter struct {
	JobID   string
	Kind    synthetic.Kind
	Outcome synthetic.Outcome
	After   int64
	Before  *int64
	Limit   int
}
type RunPage struct {
	Runs      []synthetic.Run `json:"runs"`
	Truncated bool            `json:"truncated"`
}
