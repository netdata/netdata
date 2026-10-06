// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// verifyStartup precedes SDK NewLog, which may archive/reuse an existing active
// file. Only finalized current-schema archives from this writer lifecycle are
// trusted without a whole-graph check. External modification is not supported.
func (s *Store) verifyStartup(ctx context.Context) error {
	paths, err := s.journalPaths(ctx, true)
	if err != nil {
		return err
	}
	for _, path := range paths {
		active := filepath.Base(path) == "dem.journal"
		if active {
			if err := journal.VerifyIndex(ctx, path); err != nil {
				return fmt.Errorf("verify active history %q: %w", path, err)
			}
		}
		file, err := openSnapshot(ctx, path)
		if err != nil {
			return err
		}
		empty := file.EntryCount() == 0
		if err := file.Close(); err != nil {
			return err
		}
		// No schema postings means an empty archive cannot establish encoder
		// provenance. Strict verification can establish that nothing was omitted.
		if !active && empty {
			if err := journal.VerifyIndex(ctx, path); err != nil {
				return fmt.Errorf("verify empty history archive %q: %w", path, err)
			}
		}
	}
	return ctx.Err()
}
