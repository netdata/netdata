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

// History is redacted at ingestion with the site redactor of that time. Output
// applies the current site redactor while the site is active, and the
// credential patterns once it is retired, so records written by earlier builds
// or pattern sets get the current patterns too. A failed visit leaves the map
// partial, so callers return no rows: an active site missing from it would get
// only the credential patterns, not its configured values.
func (s *Service) redactors(ctx context.Context) (map[string]*redact.Redactor, error) {
	out := map[string]*redact.Redactor{}
	err := s.visit(ctx, "", func(key string, site *rumregistry.Site) { out[key] = siteRedactor(site) })
	return out, err
}

// outputRedactor redacts a history row of site, given the active sites'
// redactors; a retired site's rows get the credential patterns.
func outputRedactor(active map[string]*redact.Redactor, site string) *redact.Redactor {
	if r := active[site]; r != nil {
		return r
	}
	return redact.Patterns
}

func (s *Service) Sessions(ctx context.Context, f SessionFilter) ([]Session, error) {
	rows, err := s.history.QuerySessions(ctx, f)
	if err != nil {
		return nil, err
	}
	redactors, err := s.redactors(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		r := &rows[i]
		redact := outputRedactor(redactors, r.Site)
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
	return rows, nil
}
func (s *Service) SessionEvents(ctx context.Context, site, session, vnodeGUID string) ([]SessionEvent, error) {
	// Copy the bounded active ring before journal I/O, releasing leases before any I/O.
	// A retiring generation can then flush while this query reads persisted data.
	var live []history.SessionEventRecord
	redactors := map[string]*redact.Redactor{}
	err := s.visit(ctx, site, func(key string, runtime *rumregistry.Site) {
		redact := siteRedactor(runtime)
		redactors[key] = redact
		events, _ := runtime.Aggregator.SessionEvents(session)
		for _, event := range events {
			if vnodeGUID != "" && event.VnodeGUID != vnodeGUID {
				continue
			}
			row := history.SessionEventOf(event)
			redactSessionEvent(&row, redact)
			live = append(live, row)
		}
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.history.QuerySessionEvents(ctx, site, session, vnodeGUID)
	if err != nil {
		return nil, err
	}
	// Match occurrences, not just values: two identical events in a beacon
	// remain two after the same events have also reached the history writer.
	// Both sources project through history.SessionEventOf, so equal events compare equal.
	overlap := make(map[history.SessionEventRecord]int, len(rows))
	for i := range rows {
		redactSessionEvent(&rows[i], outputRedactor(redactors, rows[i].Site))
		overlap[rows[i]]++
	}
	for _, row := range live {
		if overlap[row] > 0 {
			overlap[row]--
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ObservedUS < rows[j].ObservedUS })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}
func redactSessionEvent(row *history.SessionEventRecord, redact *redact.Redactor) {
	row.UserID = redact.Apply(row.UserID)
	row.Type = redact.Apply(row.Type)
	row.Page = redact.Apply(row.Page)
	row.View = redact.Apply(row.View)
	row.Text = redact.Apply(row.Text)
}

func (s *Service) Errors(ctx context.Context, f ErrorFilter) ([]ErrorGroup, error) {
	rows, err := s.history.QueryErrors(ctx, f)
	if err != nil {
		return nil, err
	}
	redactors, err := s.redactors(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		r := &rows[i]
		redact := outputRedactor(redactors, r.Site)
		r.Type = redact.Apply(r.Type)
		r.Message = redact.Apply(r.Message)
		r.SampleStack = redact.Apply(r.SampleStack)
		r.TopPage = redact.Apply(r.TopPage)
		for j := range r.Browsers {
			r.Browsers[j] = redact.Apply(r.Browsers[j])
		}
	}
	return rows, nil
}
