// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// scan selects receipt-time buckets, then checks inclusive whole-second bounds.
func (s *Store) scan(ctx context.Context, site string, after, before int64, visit func(EventRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if after < 0 || before < after {
		return fmt.Errorf("invalid RUM observation range")
	}
	reader, closeReader, err := s.journal.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	if reader == nil {
		return ctx.Err()
	}
	return reader.VisitRange("rum", after, before, func(entry *journal.SnapshotEntry) error {
		record, rum, err := decodeEvent(entry)
		if err != nil {
			return err
		}
		if rum && (site == "" || record.Site == site) && record.ObservedUS/1_000_000 >= after && record.ObservedUS/1_000_000 <= before {
			visit(record)
		}
		return nil
	})
}

// matchSession reads identity postings across all retained time.
func (s *Store) matchSession(ctx context.Context, site, session string, visit func(EventRecord)) error {
	reader, closeReader, err := s.journal.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	if reader == nil {
		return ctx.Err()
	}
	return reader.VisitMatch("DEM_SESSION_ID", session, func(entry *journal.SnapshotEntry) error {
		record, rum, err := decodeEvent(entry)
		if err != nil {
			return err
		}
		if rum && record.SessionID == session && (site == "" || record.Site == site) {
			visit(record)
		}
		return nil
	})
}
