// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// EnforceHistoryRetention applies SDK whole-file policies. Closing and
// lazily reopening archives idle activity, so age expiry also runs without
// new events. The active file remains protected, so maxBytes is not a hard cap.
// MaxBytes measures committed journal bytes, excluding filesystem preallocation.
// Age is measured from each file's saved-time head, not from each event.
func (s *Store) EnforceHistoryRetention(ctx context.Context, days int, maxBytes int64) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return os.ErrClosed
	}
	if s.failure != nil {
		return s.failure
	}
	policy := journal.RetentionPolicy{}
	if days > 0 {
		const maxDays = int64(^uint64(0)>>1) / int64(24*time.Hour)
		if int64(days) > maxDays {
			return fmt.Errorf("history retention days overflow: %d", days)
		}
		policy = policy.WithMaxAge(time.Duration(days) * 24 * time.Hour)
	}
	if maxBytes > 0 {
		policy = policy.WithMaxBytes(uint64(maxBytes))
	}
	// Reopening applies the new policy; closing must not run stale limits.
	if s.log != nil {
		if err := s.log.CloseWithoutRetention(); err != nil {
			return fmt.Errorf("archive history journal: %w", s.recordFailure(err))
		}
		s.log = nil
	}
	s.config.RetentionPolicy = policy
	// NewLog derives rotation thresholds from the retention budget. Keep its
	// writer lazy, and own the log before retention can fail. Idle sweeps need
	// no new active file; the next append creates one with a fresh time head.
	config := s.config
	config.OpenMode = journal.LogOpenLazy
	log, err := journal.NewLog(s.root, config)
	if err != nil {
		return fmt.Errorf("reopen history journal: %w", err)
	}
	s.log = log
	if err := s.log.EnforceRetention(); err != nil {
		return fmt.Errorf("enforce history retention: %w", err)
	}
	return nil
}
