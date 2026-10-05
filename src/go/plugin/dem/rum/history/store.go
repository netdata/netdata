// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
)

// Store encodes and queries RUM records in the command-owned journal.
// It borrows the journal; only the command may close it or enforce retention.
type Store struct{ journal *demjournal.Store }

func NewStore(journal *demjournal.Store) *Store {
	return &Store{
		journal: journal,
	}
}

// AppendEvent reports whether the SDK append was attempted, including uncertain failures.
// An attempted record must never be blindly replayed.
func (s *Store) AppendEvent(ctx context.Context, r EventRecord) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return s.journal.Append(ctx, eventFields(r))
}

func (s *Store) Sync(ctx context.Context) error { return s.journal.Sync(ctx) }
