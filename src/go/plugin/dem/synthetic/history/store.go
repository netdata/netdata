// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Store encodes and queries synthetic runs in the command-owned journal.
// It borrows the journal; only the command may close it or enforce retention.
type Store struct{ store *journalstore.Store }

func NewStore(store *journalstore.Store) *Store {
	return &Store{
		store: store,
	}
}
func (s *Store) Sync(ctx context.Context) error { return s.store.Sync(ctx) }

type RunFilter struct {
	JobID     string
	VnodeGUID string
	Kind      synthetic.Kind
	Outcome   synthetic.Outcome
	After     int64
	Before    *int64
	Limit     int
}
type RunPage struct {
	Runs      []synthetic.Run `json:"runs"`
	Truncated bool            `json:"truncated"`
}
