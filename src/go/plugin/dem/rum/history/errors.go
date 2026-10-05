// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"sort"
)

func (s *Store) QueryErrors(
	ctx context.Context,
	site, fingerprint string,
	after, before int64,
) ([]ErrorGroup, error) {
	type group struct {
		record          ErrorGroup
		first, last     int64
		sessions        map[string]struct{}
		pages, browsers map[string]int
	}
	groups := make(map[[2]string]*group)
	err := s.scan(ctx, site, after, before, func(r EventRecord) {
		if r.Type != "error" || (fingerprint != "" && r.Fingerprint != fingerprint) {
			return
		}
		key := [2]string{r.Site, r.Fingerprint}
		g := groups[key]
		if g == nil {
			g = &group{
				record: ErrorGroup{
					Site:        r.Site,
					Fingerprint: r.Fingerprint,
					Type:        r.ErrorType,
					Message:     r.Message,
					SampleStack: r.SampleStack,
					Details:     fingerprint != "",
				},
				first: r.TSUnixUS,
				last:  r.TSUnixUS,
			}
			if g.record.Details {
				g.sessions = make(map[string]struct{})
				g.pages = make(map[string]int)
				g.browsers = make(map[string]int)
			}
			groups[key] = g
		}
		g.record.CountWindow++
		if r.TSUnixUS < g.first {
			g.first = r.TSUnixUS
			g.record.Type, g.record.Message, g.record.SampleStack = r.ErrorType, r.Message, r.SampleStack
		}
		g.last = max(g.last, r.TSUnixUS)
		if g.record.Details {
			if r.SessionID != "" {
				g.sessions[r.SessionID] = struct{}{}
			}
			if r.Page != "" {
				g.pages[r.Page]++
			}
			if r.Browser != "" {
				g.browsers[r.Browser]++
			}
		}
	})
	if err != nil {
		return nil, err
	}
	out := make([]ErrorGroup, 0, len(groups))
	for _, g := range groups {
		g.record.FirstSeen, g.record.LastSeen = g.first/1_000_000, g.last/1_000_000
		if g.record.Details {
			g.record.SessionsAffected = len(g.sessions)
			if pages := topValues(g.pages, 1); len(pages) != 0 {
				g.record.TopPage = pages[0]
			}
			g.record.Browsers = topValues(g.browsers, 3)
		}
		out = append(out, g.record)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	return out, ctx.Err()
}

func topValues(counts map[string]int, limit int) []string {
	values := make([]string, 0, len(counts))
	for value := range counts {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] != counts[values[j]] {
			return counts[values[i]] > counts[values[j]]
		}
		return values[i] < values[j]
	})
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}
