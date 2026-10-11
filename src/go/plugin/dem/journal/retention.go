// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// MinHistoryBytes is the SDK's minimum file allocation, including preallocation.
const MinHistoryBytes = 8 << 20

// EnforceHistoryRetention applies one policy to all retained machine identities.
// Full file lengths count toward the allowance. Whole files expire by their newest
// saved record; ordinary sweeps preserve the active file. This is neither an exact
// record TTL nor a hard disk cap. Safe cleanup errors do not fail healthy appends.
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
	if days < 1 || days > 365 {
		return fmt.Errorf("history retention days must be between 1 and 365: %d", days)
	}
	if maxBytes < MinHistoryBytes {
		return fmt.Errorf("history retention max_bytes must be at least %d: %d", MinHistoryBytes, maxBytes)
	}
	policy := journal.RetentionPolicy{}.
		WithMaxAge(time.Duration(days) * 24 * time.Hour).
		WithMaxBytes(uint64(maxBytes))
	if err := s.log.SetRootRetentionPolicy(policy); err != nil {
		return fmt.Errorf("set history retention policy: %w", s.recordFailure(err))
	}
	if _, err := s.log.MaintainRootRetention(time.Now()); err != nil {
		return fmt.Errorf("enforce history retention: %w", s.recordFailure(err))
	}
	return nil
}
