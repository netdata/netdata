// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"

	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

type livePosition struct {
	Generation string `json:"g"`
	Sequence   uint64 `json:"s"`
}
type liveCursor struct {
	Version int                     `json:"v"`
	Sites   map[string]livePosition `json:"sites"`
}

func (s *Service) Live(ctx context.Context, filter, after string, limit int) ([]LiveEvent, string, error) {
	cursor := liveCursor{
		Version: 1,
		Sites:   map[string]livePosition{},
	}
	if after != "" {
		data, err := base64.RawURLEncoding.DecodeString(after)
		if err != nil {
			return nil, "", InvalidArgument{
				Message: "invalid live cursor",
			}
		}
		if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || cursor.Sites == nil {
			return nil, "", InvalidArgument{
				Message: "invalid live cursor",
			}
		}
	}
	if limit <= 0 {
		return nil, "", errors.New("live row limit must be positive")
	}
	var rows []LiveEvent
	// Retain positions of quiet sites, and discard retired identities. A caller
	// may change its filter without advancing any unobserved site's position.
	active := map[string]bool{}
	for _, key := range s.registry.Keys() {
		active[key] = true
	}
	for key := range cursor.Sites {
		if !active[key] {
			delete(cursor.Sites, key)
		}
	}
	err := s.visit(ctx, filter, func(key string, site *rumregistry.Site) {
		pos := cursor.Sites[key]
		if pos.Generation != site.Generation {
			pos = livePosition{
				Generation: site.Generation,
			}
		}
		cursor.Sites[key] = pos
		batch, _ := site.Aggregator.Live(pos.Sequence, limit)
		redact := siteRedactor(site)
		for _, row := range batch {
			row.Page = redact.Apply(row.Page)
			row.City = redact.Apply(row.City)
			row.Country = redact.Apply(row.Country)
			row.Browser = redact.Apply(row.Browser)
			row.Device = redact.Apply(row.Device)
			rows = append(rows, LiveEvent{
				LiveObservation: LiveObservation(row),
				Generation:      site.Generation,
			})
		}
	})
	if err != nil {
		return nil, "", err
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].TS.Equal(rows[j].TS) {
			return rows[i].TS.Before(rows[j].TS)
		}
		if rows[i].Site != rows[j].Site {
			return rows[i].Site < rows[j].Site
		}
		return rows[i].Seq < rows[j].Seq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	for _, row := range rows {
		cursor.Sites[row.Site] = livePosition{
			Generation: row.Generation,
			Sequence:   row.Seq,
		}
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil, "", err
	}
	return rows, base64.RawURLEncoding.EncodeToString(data), nil
}
