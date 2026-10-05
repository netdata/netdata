// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"context"
	"slices"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
)

// History is sanitized before persistence, including for retired sites. Active
// redactors additionally guard output using the current configured credentials.
func (s *Service) redactors(ctx context.Context) (map[string]*redact.Redactor, error) {
	out := map[string]*redact.Redactor{}
	err := s.visit(ctx, "", func(key string, site *rumregistry.Site) { out[key] = siteRedactor(site) })
	return out, err
}
func (s *Service) Sessions(ctx context.Context, site, userID string, after, before int64) ([]Session, error) {
	rows, err := s.history.QuerySessions(ctx, site, userID, after, before, 2000)
	if err != nil {
		return nil, err
	}
	redactors, err := s.redactors(ctx)
	for i := range rows {
		r := &rows[i]
		if redact := redactors[r.Site]; redact != nil {
			for j := range r.UserIDs {
				r.UserIDs[j] = redact.Apply(r.UserIDs[j])
			}
			sort.Strings(r.UserIDs)
			r.UserIDs = slices.Compact(r.UserIDs)
			r.Version = redact.Apply(r.Version)
			r.Browser = redact.Apply(r.Browser)
			r.Device = redact.Apply(r.Device)
			r.Country = redact.Apply(r.Country)
			r.EntryPage = redact.Apply(r.EntryPage)
			r.LastPage = redact.Apply(r.LastPage)
		}
	}
	out := make([]Session, len(rows))
	for i, row := range rows {
		out[i] = Session(row)
	}
	return out, err
}
func (s *Service) SessionEvents(ctx context.Context, site, session string) ([]SessionEvent, error) {
	// Copy the bounded active ring before journal I/O, releasing leases before any I/O.
	// A retiring generation can then flush while this query reads persisted data.
	var live []history.SessionEventRecord
	redactors := map[string]*redact.Redactor{}
	err := s.visit(ctx, site, func(key string, runtime *rumregistry.Site) {
		redact := siteRedactor(runtime)
		redactors[key] = redact
		events, _ := runtime.Aggregator.SessionEvents(session)
		for _, event := range events {
			row := history.SessionEventRecord{
				ExperienceID: event.ExperienceID,
				View:         event.View,
				ViewID:       event.ViewID,
				MetricID:     event.MetricID,
				Revision:     event.Revision,

				Site:      key,
				SessionID: session,
				TSUnixUS:  event.TS.UnixMicro(),
				Type:      event.Type,
				Page:      event.Page,
				Text:      event.Text,
				TraceID:   event.TraceID,
				UserID:    event.UserID,
			}
			redactSessionEvent(&row, redact)
			live = append(live, row)
		}
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.history.QuerySessionEvents(ctx, site, session)
	if err != nil {
		return nil, err
	}
	// Match occurrences, not just values: two identical events in a beacon
	// remain two after the same events have also reached the history writer.
	overlap := make(map[history.SessionEventRecord]int, len(rows))
	for i := range rows {
		if redact := redactors[rows[i].Site]; redact != nil {
			redactSessionEvent(&rows[i], redact)
		}
		overlap[rows[i]]++
	}
	for _, row := range live {
		if overlap[row] > 0 {
			overlap[row]--
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].TSUnixUS < rows[j].TSUnixUS })
	out := make([]SessionEvent, len(rows))
	for i, row := range rows {
		out[i] = SessionEvent(row)
	}
	return out, ctx.Err()
}
func redactSessionEvent(row *history.SessionEventRecord, redact *redact.Redactor) {
	row.UserID = redact.Apply(row.UserID)
	row.Type = redact.Apply(row.Type)
	row.Page = redact.Apply(row.Page)
	row.View = redact.Apply(row.View)
	row.Text = redact.Apply(row.Text)
}

func (s *Service) Errors(
	ctx context.Context,
	site, fingerprint string,
	after, before int64,
) ([]ErrorGroup, error) {
	rows, err := s.history.QueryErrors(ctx, site, fingerprint, after, before)
	if err != nil {
		return nil, err
	}
	redactors, err := s.redactors(ctx)
	for i := range rows {
		r := &rows[i]
		if redact := redactors[r.Site]; redact != nil {
			r.Type = redact.Apply(r.Type)
			r.Message = redact.Apply(r.Message)
			r.SampleStack = redact.Apply(r.SampleStack)
			r.TopPage = redact.Apply(r.TopPage)
			for j := range r.Browsers {
				r.Browsers[j] = redact.Apply(r.Browsers[j])
			}
		}
	}
	out := make([]ErrorGroup, len(rows))
	for i, row := range rows {
		out[i] = ErrorGroup(row)
	}
	return out, err
}
