// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
)

// scan uses unfiltered Step so cancellation is checked for every examined row,
// including nonmatches. The SDK's filtered Step may scan many rows internally.
func (s *Store) scan(ctx context.Context, site string, after, before int64, visit func(EventRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if before < after || before < 0 {
		return nil
	}
	reader, closeReader, err := s.journal.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	if reader == nil {
		return ctx.Err()
	}
	if after > 0 {
		if uint64(after) > ^uint64(0)/1_000_000 {
			return nil
		}
		if err := reader.SeekRealtimeUsec(uint64(after) * 1_000_000); err != nil {
			return err
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		more, err := reader.Step()
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
		saved, err := reader.GetRealtimeUsec()
		if err != nil {
			return err
		}
		seconds := saved / 1_000_000
		if seconds > uint64(before) {
			continue
		}
		if after > 0 && seconds < uint64(after) {
			continue
		}
		record, rum, err := decodeEvent(reader)
		if err != nil {
			return err
		}
		if !rum || (site != "" && record.Site != site) {
			continue
		}
		visit(record)
	}
}
