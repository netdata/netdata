// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"sort"
)

func (s *Store) QuerySessions(
	ctx context.Context,
	site string,
	after, before int64,
	limit int,
) ([]SessionRecord, error) {
	type summary struct {
		record      SessionRecord
		first, last int64
	}
	groups := make(map[[2]string]*summary)
	err := s.scan(ctx, site, after, before, func(r EventRecord) {
		if r.SessionID == "" {
			return
		}
		key := [2]string{r.Site, r.SessionID}
		g := groups[key]
		if g == nil {
			g = &summary{
				record: SessionRecord{
					Site:      r.Site,
					SessionID: r.SessionID,
					EntryPage: r.Page,
				},
				first: r.TSUnixUS,
				last:  r.TSUnixUS,
			}
			groups[key] = g
		}
		if r.TSUnixUS < g.first {
			g.first = r.TSUnixUS
			g.record.EntryPage = r.Page
		}
		if r.TSUnixUS >= g.last {
			g.last = r.TSUnixUS
			g.record.LastPage = r.Page
			g.record.Browser, g.record.Device = r.Browser, r.Device
			g.record.Country, g.record.City = r.Country, r.City
			g.record.Version, g.record.UserID = r.Version, r.UserID
		}
		switch r.Type {
		case "pageview":
			g.record.Pageviews++
		case "error":
			g.record.Errors++
		case "frustration":
			g.record.Frustrations++
		}
	})
	if err != nil {
		return nil, err
	}
	out := make([]SessionRecord, 0, len(groups))
	for _, g := range groups {
		g.record.StartedAt, g.record.LastAt = g.first/1_000_000, g.last/1_000_000
		out = append(out, g.record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastAt != out[j].LastAt {
			return out[i].LastAt > out[j].LastAt
		}
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].SessionID < out[j].SessionID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, ctx.Err()
}

// QuerySessionEvents returns the full retained timeline in original-time
// order. Activity-only presence records contribute to summaries, not timelines.
func (s *Store) QuerySessionEvents(ctx context.Context, site, sessionID string) ([]SessionEventRecord, error) {
	var out []SessionEventRecord
	err := s.scan(ctx, site, 0, int64(^uint64(0)>>1), func(r EventRecord) {
		if r.SessionID != sessionID || r.Type == "activity" {
			return
		}
		out = append(
			out,
			SessionEventRecord{
				Site:      r.Site,
				SessionID: r.SessionID,
				TSUnixUS:  r.TSUnixUS,
				Type:      r.Type,
				Page:      r.Page,
				Text:      r.Text,
				TraceID:   r.TraceID,
			},
		)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixUS < out[j].TSUnixUS })
	return out, ctx.Err()
}
