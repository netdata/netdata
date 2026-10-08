// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"slices"
	"sort"
)

// QuerySessions matches an optional exact user ID in the selected receipt-time range
// before limiting results, while summarizing every selected event in a matching session.
func (s *Store) QuerySessions(
	ctx context.Context,
	site, userID string,
	after, before int64,
	limit int,
) ([]SessionRecord, error) {
	type summary struct {
		record      SessionRecord
		first, last int64
		userIDs     map[string]struct{}
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
				first: r.ObservedUS,
				last:  r.ObservedUS,
			}
			groups[key] = g
		}
		if r.ObservedUS < g.first {
			g.first = r.ObservedUS
			g.record.EntryPage = r.Page
		}
		if r.ObservedUS >= g.last {
			g.last = r.ObservedUS
			g.record.LastPage = r.Page
			g.record.Browser, g.record.Device = r.Browser, r.Device
			g.record.Country = r.Country
			g.record.Version = r.Version
		}
		if r.UserID != "" {
			if g.userIDs == nil {
				g.userIDs = make(map[string]struct{})
			}
			g.userIDs[r.UserID] = struct{}{}
		}
		switch r.Type {
		case "pageview":
			g.record.Pageviews++
		case "view":
			g.record.ApplicationViews++
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
		if userID != "" {
			if _, ok := g.userIDs[userID]; !ok {
				continue
			}
		}
		g.record.UserIDs = make([]string, 0, len(g.userIDs))
		for id := range g.userIDs {
			g.record.UserIDs = append(g.record.UserIDs, id)
		}
		slices.Sort(g.record.UserIDs)
		g.record.StartedAt, g.record.LastAt = g.first/1_000_000, g.last/1_000_000
		g.record.LastObservedUS = g.last
		out = append(out, g.record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastObservedUS != out[j].LastObservedUS {
			return out[i].LastObservedUS > out[j].LastObservedUS
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

// QuerySessionEvents returns the full retained timeline in Agent receipt-time
// order. Activity-only presence records contribute to summaries, not timelines.
func (s *Store) QuerySessionEvents(ctx context.Context, site, sessionID string) ([]SessionEventRecord, error) {
	var out []SessionEventRecord
	err := s.matchSession(ctx, site, sessionID, func(r EventRecord) {
		if r.Type == "activity" {
			return
		}
		out = append(
			out,
			SessionEventRecord{
				ExperienceID: r.ExperienceID,
				View:         r.View,
				ViewID:       r.ViewID,
				MetricID:     r.MetricID,
				Revision:     r.Revision,

				Site:       r.Site,
				SessionID:  r.SessionID,
				ObservedUS: r.ObservedUS,
				Type:       r.Type,
				Page:       r.Page,
				Text:       r.Text,
				TraceID:    r.TraceID,
				UserID:     r.UserID,
			},
		)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ObservedUS < out[j].ObservedUS })
	return out, ctx.Err()
}
