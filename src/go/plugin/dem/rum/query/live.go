// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"container/heap"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

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
	var streams liveStreams
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
		stream := make([]LiveEvent, 0, len(batch))
		for _, row := range batch {
			row.Page = redact.Apply(row.Page)
			row.City = redact.Apply(row.City)
			row.Country = redact.Apply(row.Country)
			row.Browser = redact.Apply(row.Browser)
			row.Device = redact.Apply(row.Device)
			stream = append(stream, LiveEvent{
				LiveObservation: LiveObservation(row),
				Generation:      site.Generation,
			})
		}
		if len(stream) > 0 {
			streams = append(streams, stream)
		}
	})
	if err != nil {
		return nil, "", err
	}
	// Receipt timestamps can precede ingestion in concurrent HTTP requests.
	// Merge stream heads so every returned site's rows remain a sequence prefix.
	heap.Init(&streams)
	var rows []LiveEvent
	for len(streams) > 0 && len(rows) < limit {
		row := streams[0][0]
		rows = append(rows, row)
		cursor.Sites[row.Site] = livePosition{
			Generation: row.Generation,
			Sequence:   row.Seq,
		}
		if len(streams[0]) == 1 {
			heap.Pop(&streams)
		} else {
			streams[0] = streams[0][1:]
			heap.Fix(&streams, 0)
		}
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil, "", err
	}
	return rows, base64.RawURLEncoding.EncodeToString(data), nil
}

// Each stream is ordered by its site's sequence; only its head is eligible.
type liveStreams [][]LiveEvent

func (s liveStreams) Len() int { return len(s) }
func (s liveStreams) Less(i, j int) bool {
	a, b := s[i][0], s[j][0]
	if !a.TS.Equal(b.TS) {
		return a.TS.Before(b.TS)
	}
	if a.Site != b.Site {
		return a.Site < b.Site
	}
	return a.Seq < b.Seq
}
func (s liveStreams) Swap(i, j int)    { s[i], s[j] = s[j], s[i] }
func (s *liveStreams) Push(stream any) { *s = append(*s, stream.([]LiveEvent)) }
func (s *liveStreams) Pop() any {
	last := len(*s) - 1
	stream := (*s)[last]
	(*s)[last] = nil
	*s = (*s)[:last]
	return stream
}
