// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/vnodeguid"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
)

// Store encodes and queries RUM records in the command-owned journal.
// It borrows the journal; only the command may close it or enforce retention.
type Store struct{ store *journalstore.Store }

func NewStore(store *journalstore.Store) *Store {
	return &Store{
		store: store,
	}
}

// AppendEvent reports whether the SDK append was attempted, including uncertain failures.
// An attempted record must never be blindly replayed.
func (s *Store) AppendEvent(ctx context.Context, r EventRecord) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Validate captured vnode identity at admission; decoded records retain their stored attribution.
	if err := vnodeguid.Validate(r.VnodeGUID); err != nil {
		return false, err
	}
	if err := validateEvent(r); err != nil {
		return false, err
	}
	return s.store.Append(ctx, eventFields(r))
}

func (s *Store) Sync(ctx context.Context) error { return s.store.Sync(ctx) }
