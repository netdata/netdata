// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"fmt"
	"os"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// Append reports attempted once SDK Append is called, including errors
// with uncertain disk outcomes. Such entries must never be blindly replayed.
// Cancellation interrupts admission only; an admitted SDK disk call runs to
// completion. Append publishes for readers; Sync/Close supplies the fsync.
func (s *Store) Append(ctx context.Context, fields []journal.Field) (attempted bool, err error) {
	if err := s.acquire(ctx); err != nil {
		return false, err
	}
	defer s.release()
	if s.closed {
		return false, os.ErrClosed
	}
	if s.log == nil {
		return false, fmt.Errorf("history journal unavailable after reopen failure")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, s.log.Append(fields, s.host.EntryOptions())
}

// Sync flushes admitted entries to disk. Cancellation interrupts admission, not
// the SDK disk call. A failed sync must not trigger replay of appended events.
func (s *Store) Sync(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return os.ErrClosed
	}
	if s.log == nil {
		return fmt.Errorf("history journal unavailable after reopen failure")
	}
	return s.log.Sync()
}
