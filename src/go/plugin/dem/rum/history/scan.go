// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// Filter selects retained events by Agent receipt time.
type Filter struct {
	Site      string // "" selects every site
	VnodeGUID string // "" selects every vnode
	// After and Before are inclusive Unix seconds.
	After  int64
	Before int64
}

// scan selects receipt-time buckets, then checks inclusive whole-second bounds.
func (s *Store) scan(ctx context.Context, f Filter, visit func(EventRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.After < 0 || f.Before < f.After {
		return fmt.Errorf("invalid RUM observation range")
	}
	reader, closeReader, err := s.store.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	var previous EventRecord
	return reader.VisitRange(journalstore.KindRUM, f.After, f.Before, func(entry *journal.SnapshotEntry) error {
		record, rum, err := decodeEvent(entry, &previous)
		if err != nil {
			return err
		}
		previous = record
		if rum && (f.VnodeGUID == "" || record.VnodeGUID == f.VnodeGUID) && (f.Site == "" || record.Site == f.Site) &&
			record.ObservedUS/1_000_000 >= f.After &&
			record.ObservedUS/1_000_000 <= f.Before {
			visit(record)
		}
		return nil
	})
}

// matchSession reads identity postings across all retained time.
func (s *Store) matchSession(ctx context.Context, site, session, vnodeGUID string, visit func(EventRecord)) error {
	reader, closeReader, err := s.store.OpenReader(ctx)
	if err != nil {
		return err
	}
	defer closeReader()
	var previous EventRecord
	return reader.VisitMatch("DEM_SESSION_ID", session, func(entry *journal.SnapshotEntry) error {
		record, rum, err := decodeEvent(entry, &previous)
		if err != nil {
			return err
		}
		previous = record
		if rum && (vnodeGUID == "" || record.VnodeGUID == vnodeGUID) && record.SessionID == session &&
			(site == "" || record.Site == site) {
			visit(record)
		}
		return nil
	})
}
