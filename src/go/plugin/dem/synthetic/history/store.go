// SPDX-License-Identifier: GPL-3.0-or-later

// Package history persists synthetic run start and completion records in the process-owned DEM journal and reduces
// them back into runs for queries.
package history

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Store encodes and queries synthetic runs in the process-owned journal.
// It borrows the journal; only the plugin process (dem.Process) may close it or enforce retention.
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
	Runs      []synthetic.Run
	Truncated bool
}
