// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
)

type source struct {
	hub     *runtimehub.Hub
	history *store.Store
}

var _ rumfunc.Deps = (*source)(nil)

func (s *source) Receiver() runtimehub.Availability { return s.hub.Availability() }

// visit holds a lease only while copying domain snapshots. Retirement can
// revoke future admission without exposing a collector or desired config.
func (s *source) visit(ctx context.Context, filter string, fn func(string, *runtimehub.Site)) error {
	keys := s.hub.Keys()
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if filter != "" && key != filter {
			continue
		}
		site, leaseCtx, release, ok := s.hub.AcquireSite(key)
		if !ok {
			continue
		}
		if leaseCtx.Err() == nil {
			fn(key, site)
		}
		release()
	}
	return ctx.Err()
}
func siteRedactor(site *runtimehub.Site) *secrets.Redactor {
	if site.Redactor != nil {
		return site.Redactor
	}
	return secrets.NewRedactor()
}
func (s *source) Sites(ctx context.Context) ([]rumfunc.Observation, error) {
	var out []rumfunc.Observation
	state := s.Receiver()
	err := s.visit(ctx, "", func(key string, site *runtimehub.Site) {
		cfg := site.Route.Config()
		fallback := state.PublicURL
		if fallback == "" && state.Listen != "" {
			fallback = cfg.PublicBase(state.Listen, state.TLS)
		}
		// Explicit receiver configuration takes effect immediately, even while
		// the site's last successful probe still describes an older proxy URL.
		publicBase := strings.TrimRight(state.PublicURL, "/")
		if cfg.PublicURL != "" || publicBase == "" {
			publicBase = site.Route.PublicBase(fallback)
		}
		reach, _ := site.Route.Reachability()
		snippet, _ := site.Route.Snippet()
		rejected, _ := site.Route.LastRejectedOrigin()
		redact := siteRedactor(site)
		reach.Error = redact.Apply(reach.Error)
		snippet.Detail = redact.Apply(snippet.Detail)
		rejected.Origin = redact.Apply(rejected.Origin)
		cfg.DisplayName = redact.Apply(cfg.DisplayName)
		cfg.AllowedOrigins = append([]string(nil), cfg.AllowedOrigins...)
		for i := range cfg.AllowedOrigins {
			cfg.AllowedOrigins[i] = redact.Apply(cfg.AllowedOrigins[i])
		}
		out = append(
			out,
			rumfunc.Observation{
				Config:     cfg,
				Activity:   site.Aggregator.Activity(),
				Reach:      reach,
				Snippet:    snippet,
				Rejected:   rejected,
				PublicBase: redact.Apply(publicBase),
			},
		)
	})
	return out, err
}
func (s *source) Pages(ctx context.Context, filter string) ([]agg.PageInfo, error) {
	var out []agg.PageInfo
	err := s.visit(ctx, filter, func(key string, site *runtimehub.Site) {
		rows := site.Aggregator.Pages()
		redact := siteRedactor(site)
		for i := range rows {
			r := &rows[i]
			r.Page = redact.Apply(r.Page)
			r.LCPElement = redact.Apply(r.LCPElement)
			r.INPElement = redact.Apply(r.INPElement)
			r.CLSElement = redact.Apply(r.CLSElement)
		}
		out = append(out, rows...)
	})
	return out, err
}

type livePosition struct {
	Generation string `json:"g"`
	Sequence   uint64 `json:"s"`
}
type liveCursor struct {
	Version int                     `json:"v"`
	Sites   map[string]livePosition `json:"sites"`
}

func (s *source) Live(ctx context.Context, filter, after string, limit int) ([]rumfunc.LiveEvent, string, error) {
	cursor := liveCursor{
		Version: 1,
		Sites:   map[string]livePosition{},
	}
	if after != "" {
		data, err := base64.RawURLEncoding.DecodeString(after)
		if err != nil {
			return nil, "", rumfunc.InvalidArgument{
				Message: "invalid live cursor",
			}
		}
		if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || cursor.Sites == nil {
			return nil, "", rumfunc.InvalidArgument{
				Message: "invalid live cursor",
			}
		}
	}
	if limit <= 0 {
		return nil, "", errors.New("live row limit must be positive")
	}
	var rows []rumfunc.LiveEvent
	// Retain positions of quiet sites, and discard retired identities. A caller
	// may change its filter without advancing any unobserved site's position.
	active := map[string]bool{}
	for _, key := range s.hub.Keys() {
		active[key] = true
	}
	for key := range cursor.Sites {
		if !active[key] {
			delete(cursor.Sites, key)
		}
	}
	err := s.visit(ctx, filter, func(key string, site *runtimehub.Site) {
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
			rows = append(rows, rumfunc.LiveEvent{
				LiveRow:    row,
				Generation: site.Generation,
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

// History is sanitized before persistence, including for retired sites. Active
// redactors additionally guard output using the current configured credentials.
func (s *source) redactors(ctx context.Context) (map[string]*secrets.Redactor, error) {
	out := map[string]*secrets.Redactor{}
	err := s.visit(ctx, "", func(key string, site *runtimehub.Site) { out[key] = siteRedactor(site) })
	return out, err
}
func (s *source) Sessions(ctx context.Context, site string, after, before int64) ([]store.RumSessionRecord, error) {
	rows, err := s.history.QueryRumSessions(ctx, site, after, before, 2000)
	if err != nil {
		return nil, err
	}
	redactors, err := s.redactors(ctx)
	for i := range rows {
		r := &rows[i]
		if redact := redactors[r.Site]; redact != nil {
			r.UserID = redact.Apply(r.UserID)
			r.Version = redact.Apply(r.Version)
			r.Browser = redact.Apply(r.Browser)
			r.Device = redact.Apply(r.Device)
			r.Country = redact.Apply(r.Country)
			r.City = redact.Apply(r.City)
			r.EntryPage = redact.Apply(r.EntryPage)
			r.LastPage = redact.Apply(r.LastPage)
		}
	}
	return rows, err
}
func (s *source) SessionEvents(ctx context.Context, site, session string) ([]store.RumSessionEventRecord, error) {
	// Copy the bounded active ring before journal I/O, releasing leases before any I/O.
	// A retiring generation can then flush while this query reads persisted data.
	var live []store.RumSessionEventRecord
	redactors := map[string]*secrets.Redactor{}
	err := s.visit(ctx, site, func(key string, runtime *runtimehub.Site) {
		redact := siteRedactor(runtime)
		redactors[key] = redact
		events, _ := runtime.Aggregator.SessionEvents(session)
		for _, event := range events {
			row := store.RumSessionEventRecord{
				Site:      key,
				SessionID: session,
				TSUnixUS:  event.TS.UnixMicro(),
				Type:      event.Type,
				Page:      event.Page,
				Text:      event.Text,
				TraceID:   event.TraceID,
			}
			redactSessionEvent(&row, redact)
			live = append(live, row)
		}
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.history.QueryRumSessionEvents(ctx, site, session)
	if err != nil {
		return nil, err
	}
	// Match occurrences, not just values: two identical events in a beacon
	// remain two after the same events have also reached the history writer.
	overlap := make(map[store.RumSessionEventRecord]int, len(rows))
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
	return rows, ctx.Err()
}
func redactSessionEvent(row *store.RumSessionEventRecord, redact *secrets.Redactor) {
	row.Type = redact.Apply(row.Type)
	row.Page = redact.Apply(row.Page)
	row.Text = redact.Apply(row.Text)
}

func (s *source) Errors(
	ctx context.Context,
	site, fingerprint string,
	after, before int64,
) ([]store.RumErrorAgg, error) {
	rows, err := s.history.QueryRumErrors(ctx, site, fingerprint, after, before)
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
	return rows, err
}
