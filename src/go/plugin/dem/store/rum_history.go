// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// RumEventRecord is an immutable self-contained investigation event. TSUnixUS
// is original event time; the journal entry realtime records when it was saved.
type RumEventRecord struct {
	Site, SessionID                                 string
	TSUnixUS                                        int64
	Type, Page, Text, TraceID                       string
	Browser, Device, Country, City, Version, UserID string
	Fingerprint, ErrorType, Message, SampleStack    string
}

// RumSessionRecord summarizes only retained events selected by the saved-time
// filter. Timestamps and page/metadata ordering use original event time.
type RumSessionRecord struct {
	Site, SessionID                         string
	StartedAt, LastAt                       int64
	Pageviews, Errors                       int64
	Browser, Device, Country, City, Version string
	EntryPage, LastPage, UserID             string
	Frustrations                            int64
}

type RumSessionEventRecord struct {
	Site, SessionID           string
	TSUnixUS                  int64
	Type, Page, Text, TraceID string
}

// RumErrorAgg counts the selected retained errors. Details is true only for a
// selected fingerprint; otherwise SessionsAffected, TopPage and Browsers are
// unset and must be displayed as unavailable, not as measured zeroes.
type RumErrorAgg struct {
	Site, Fingerprint, Type, Message, SampleStack string
	CountWindow, SessionsAffected                 int
	FirstSeen, LastSeen                           int64
	TopPage                                       string
	Browsers                                      []string
	Details                                       bool
}

func eventFields(r RumEventRecord) []journal.Field {
	fields := []journal.Field{
		journal.StringField("DEM_KIND", "rum"),
		journal.StringField("DEM_TS_US", strconv.FormatInt(r.TSUnixUS, 10)),
	}
	for _, field := range []struct{ name, value string }{
		{"DEM_SITE", r.Site}, {"DEM_SESSION_ID", r.SessionID}, {"DEM_TYPE", r.Type},
		{"DEM_PAGE", r.Page}, {"DEM_TEXT", r.Text}, {"DEM_TRACE_ID", r.TraceID},
		{"DEM_BROWSER", r.Browser}, {"DEM_DEVICE", r.Device}, {"DEM_COUNTRY", r.Country},
		{"DEM_CITY", r.City}, {"DEM_VERSION", r.Version}, {"DEM_USER_ID", r.UserID},
		{"DEM_FINGERPRINT", r.Fingerprint}, {"DEM_ERROR_TYPE", r.ErrorType},
		{"DEM_MESSAGE", r.Message}, {"DEM_SAMPLE_STACK", r.SampleStack},
	} {
		if field.value != "" {
			fields = append(fields, journal.StringField(field.name, field.value))
		}
	}
	message := r.Text
	if message == "" {
		message = r.Message
	}
	if message == "" {
		message = r.Type
	}
	return append(fields, journal.StringField("MESSAGE", message))
}

// AppendRumEvent reports attempted once SDK Append is called, including errors
// with uncertain disk outcomes. Such entries must never be blindly replayed.
// Cancellation interrupts admission only; an admitted SDK disk call runs to
// completion. Append publishes for readers; Sync/Close supplies the fsync.
func (s *Store) AppendRumEvent(ctx context.Context, r RumEventRecord) (attempted bool, err error) {
	if err := s.acquire(ctx); err != nil {
		return false, err
	}
	defer s.release()
	if s.closed {
		return false, os.ErrClosed
	}
	if s.log == nil {
		return false, fmt.Errorf("history journal unavailable after reopen failure")
	}
	fields := eventFields(r)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, s.log.Append(fields, s.host.EntryOptions())
}

// Sync flushes admitted entries to disk. Cancellation interrupts admission, not
// the SDK disk call. A failed sync must not trigger replay of appended events.
func (s *Store) Sync(ctx context.Context) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return os.ErrClosed
	}
	if s.log == nil {
		return fmt.Errorf("history journal unavailable after reopen failure")
	}
	return s.log.Sync()
}

// The SDK's GetEntry materializes several complete field representations.
// Read callback-scoped payloads directly and retain only domain strings.
func decodeEvent(reader *journalSnapshot) (RumEventRecord, bool, error) {
	var r RumEventRecord
	var kind, timestamp string
	err := reader.VisitEntryPayloads(func(payload []byte) error {
		separator := bytes.IndexByte(payload, '=')
		if separator < 0 {
			return fmt.Errorf("invalid journal field payload")
		}
		value := payload[separator+1:]
		switch string(payload[:separator]) {
		case "DEM_KIND":
			kind = string(value)
		case "DEM_TS_US":
			timestamp = string(value)
		case "DEM_SITE":
			r.Site = string(value)
		case "DEM_SESSION_ID":
			r.SessionID = string(value)
		case "DEM_TYPE":
			r.Type = string(value)
		case "DEM_PAGE":
			r.Page = string(value)
		case "DEM_TEXT":
			r.Text = string(value)
		case "DEM_TRACE_ID":
			r.TraceID = string(value)
		case "DEM_BROWSER":
			r.Browser = string(value)
		case "DEM_DEVICE":
			r.Device = string(value)
		case "DEM_COUNTRY":
			r.Country = string(value)
		case "DEM_CITY":
			r.City = string(value)
		case "DEM_VERSION":
			r.Version = string(value)
		case "DEM_USER_ID":
			r.UserID = string(value)
		case "DEM_FINGERPRINT":
			r.Fingerprint = string(value)
		case "DEM_ERROR_TYPE":
			r.ErrorType = string(value)
		case "DEM_MESSAGE":
			r.Message = string(value)
		case "DEM_SAMPLE_STACK":
			r.SampleStack = string(value)
		}
		return nil
	})
	if err != nil || kind != "rum" {
		return r, false, err
	}
	r.TSUnixUS, err = strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return r, false, fmt.Errorf("invalid DEM_TS_US: %w", err)
	}
	return r, true, nil
}

// scan uses unfiltered Step so cancellation is checked for every examined row,
// including nonmatches. The SDK's filtered Step may scan many rows internally.
func (s *Store) scan(ctx context.Context, site string, after, before int64, visit func(RumEventRecord)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if before < after || before < 0 {
		return nil
	}
	reader, closeReader, err := s.openReader(ctx)
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

func (s *Store) QueryRumSessions(
	ctx context.Context,
	site string,
	after, before int64,
	limit int,
) ([]RumSessionRecord, error) {
	type summary struct {
		record      RumSessionRecord
		first, last int64
	}
	groups := make(map[[2]string]*summary)
	err := s.scan(ctx, site, after, before, func(r RumEventRecord) {
		if r.SessionID == "" {
			return
		}
		key := [2]string{r.Site, r.SessionID}
		g := groups[key]
		if g == nil {
			g = &summary{
				record: RumSessionRecord{
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
	out := make([]RumSessionRecord, 0, len(groups))
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

// QueryRumSessionEvents returns the full retained timeline in original-time
// order. Activity-only presence records contribute to summaries, not timelines.
func (s *Store) QueryRumSessionEvents(ctx context.Context, site, sessionID string) ([]RumSessionEventRecord, error) {
	var out []RumSessionEventRecord
	err := s.scan(ctx, site, 0, int64(^uint64(0)>>1), func(r RumEventRecord) {
		if r.SessionID != sessionID || r.Type == "activity" {
			return
		}
		out = append(
			out,
			RumSessionEventRecord{
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

func (s *Store) QueryRumErrors(
	ctx context.Context,
	site, fingerprint string,
	after, before int64,
) ([]RumErrorAgg, error) {
	type group struct {
		record          RumErrorAgg
		first, last     int64
		sessions        map[string]struct{}
		pages, browsers map[string]int
	}
	groups := make(map[[2]string]*group)
	err := s.scan(ctx, site, after, before, func(r RumEventRecord) {
		if r.Type != "error" || (fingerprint != "" && r.Fingerprint != fingerprint) {
			return
		}
		key := [2]string{r.Site, r.Fingerprint}
		g := groups[key]
		if g == nil {
			g = &group{
				record: RumErrorAgg{
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
	out := make([]RumErrorAgg, 0, len(groups))
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
