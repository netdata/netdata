// SPDX-License-Identifier: GPL-3.0-or-later

// Package rumfunc owns process-level RUM Functions behind admitted data snapshots.
package rumfunc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
)

type Observation struct {
	Config     config.RumSite
	Activity   agg.SiteActivity
	Reach      ingest.Reach
	Snippet    ingest.SnippetCheck
	Rejected   ingest.RejectedOrigin
	PublicBase string
}
type LiveEvent struct {
	agg.LiveRow
	Generation string
}

// Deps exposes domain snapshots/queries. The Function layer has no collectors,
// private config controller or transport/protocol implementation.
type Deps interface {
	Receiver() runtimehub.Availability
	Sites(context.Context) ([]Observation, error)
	Pages(context.Context, string) ([]agg.PageInfo, error)
	Live(context.Context, string, string, int) ([]LiveEvent, string, error)
	Sessions(context.Context, string, int64, int64) ([]store.RumSessionRecord, error)
	SessionEvents(context.Context, string, string) ([]store.RumSessionEventRecord, error)
	Errors(context.Context, string, string, int64, int64) ([]store.RumErrorAgg, error)
}

// InvalidArgument identifies caller input rejected by a domain data source.
type InvalidArgument struct{ Message string }

func (e InvalidArgument) Error() string { return e.Message }

type Handler struct {
	deps   Deps
	redact *secrets.Redactor
}

func New(deps Deps) *Handler {
	return &Handler{
		deps:   deps,
		redact: secrets.NewRedactor(),
	}
}
func (h *Handler) Cleanup(context.Context) {}
func (h *Handler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}
func (h *Handler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "raw RUM request required")
}
func Declarations() []funcapi.FunctionConfig {
	out := make([]funcapi.FunctionConfig, 0, len(methods))
	for _, spec := range methods {
		out = append(
			out,
			funcapi.FunctionConfig{
				ID:             spec.id,
				FunctionName:   spec.id,
				Name:           spec.title,
				UpdateEvery:    spec.every,
				Help:           spec.help,
				Tags:           "rum",
				RawRequest:     true,
				ManagedInfo:    true,
				HasHistory:     spec.history,
				AcceptedParams: spec.params,
			},
		)
	}
	return out
}
func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	var spec *method
	for i := range methods {
		if methods[i].id == req.Method {
			spec = &methods[i]
			break
		}
	}
	if spec == nil {
		return funcapi.NotFoundResponse(req.Method)
	}
	args := map[string]string{}
	for _, word := range req.Args {
		if req.Info && word == "info" {
			continue
		}
		key, value, ok := strings.Cut(word, ":")
		if !ok {
			return funcapi.ErrorResponse(400, "expected key:value argument")
		}
		supported := false
		for _, param := range spec.params {
			if key == param {
				supported = true
				break
			}
		}
		if !supported {
			return funcapi.ErrorResponse(400, "unsupported argument %q", key)
		}
		args[key] = value
	}
	response := map[string]any{
		"status":              200,
		"type":                "table",
		"has_history":         spec.history,
		"update_every":        spec.every,
		"help":                spec.help,
		"columns":             spec.columns,
		"data":                [][]any{},
		"default_sort_column": spec.sort,
	}
	if req.Method == "rum-sessions" || req.Method == "rum-errors" {
		sites, err := h.deps.Sites(ctx)
		if err != nil {
			return functionError(err)
		}
		var notes []string
		for _, site := range sites {
			if args["site"] != "" && args["site"] != site.Config.Key {
				continue
			}
			if note := site.Config.SamplingNote(); note != "" {
				notes = append(notes, note)
			}
		}
		if len(notes) > 0 {
			response["help"] = spec.help + ". Sampled: " + strings.Join(notes, "; ")
		}
	}
	if len(spec.params) > 0 {
		response["accepted_params"] = spec.params
	}
	if req.Info {
		return &funcapi.FunctionResponse{
			Status: 200,
			Help:   response["help"].(string),
		}
	}
	if err := ctx.Err(); err != nil {
		return funcapi.ErrorResponse(499, "request cancelled")
	}
	var rows [][]any
	var err error
	now := time.Now().Unix()
	switch req.Method {
	case "rum-sites":
		var sites []Observation
		sites, err = h.deps.Sites(ctx)
		receiver := h.deps.Receiver()
		response["collector"] = map[string]any{"public_url": receiver.PublicURL, "ingress_available": receiver.Serving}
		for _, s := range sites {
			cfg := s.Config
			a := s.Activity
			state := "enabled"
			if !receiver.Serving {
				state = "ingress_unavailable"
			} else if a.LastBeaconAgeS < 0 || a.LastBeaconAgeS > 3600 {
				state = "no_beacons"
			}
			measured, investigated := cfg.SamplingLabels()
			reach := s.Reach.State
			if reach == "" {
				reach = ingest.ReachUnknown
			}
			snippet := s.Snippet.State
			if snippet == "" {
				snippet = ingest.SnippetUnchecked
			}
			rejectAge := int64(-1)
			if !s.Rejected.At.IsZero() {
				rejectAge = now - s.Rejected.At.Unix()
			}
			beaconURL := strings.TrimRight(s.PublicBase, "/") + "/rum/" + cfg.Key + ".js"
			rows = append(
				rows,
				[]any{
					cfg.Key,
					cfg.DisplayName(),
					state,
					beaconURL,
					`<script async src="` + beaconURL + `"></script>`,
					strings.Join(cfg.AllowedOrigins, ","),
					a.BeaconsPerMin,
					a.LastBeaconAgeS,
					a.ActiveSessions,
					a.PageviewsWindow,
					a.JSErrorsWindow,
					a.RejectedPerMin,
					reach,
					h.redact.Apply(s.Reach.Error),
					measured,
					investigated,
					a.InvestigatedSessions,
					a.BotsPerMin,
					snippet,
					h.redact.Apply(s.Snippet.Detail),
					h.redact.Apply(s.Rejected.Origin),
					rejectAge,
				},
			)
		}
	case "rum-pages":
		var pages []agg.PageInfo
		pages, err = h.deps.Pages(ctx, args["site"])
		for _, p := range pages {
			rows = append(
				rows,
				[]any{
					p.Site,
					p.Page,
					p.PageviewsWindow,
					p.Sessions,
					number(p.HasLCP, p.LCPP75MS),
					number(p.HasINP, p.INPP75MS),
					number(p.HasCLS, p.CLSP75),
					p.ErrorsWindow,
					h.redact.Apply(p.LCPElement),
					h.redact.Apply(p.INPElement),
					h.redact.Apply(p.CLSElement),
					p.FrustrationWindow,
				},
			)
		}
	case "rum-live":
		var live []LiveEvent
		var next string
		live, next, err = h.deps.Live(ctx, args["site"], args["after"], 2000)
		response["next"] = next
		for _, r := range live {
			country := r.Country
			if country == "" {
				country = "unknown"
			}
			rows = append(
				rows,
				[]any{
					r.Site + ":" + r.Generation + ":" + strconv.FormatUint(r.Seq, 10),
					r.TS.UnixMicro(),
					r.Site,
					country,
					r.City,
					number(r.HasGeo, r.Lat),
					number(r.HasGeo, r.Lon),
					r.Page,
					r.Browser,
					r.Device,
					boolean(r.PageView),
					number(r.HasLCP, r.LCPMS),
					number(r.HasINP, r.INPMS),
					number(r.HasFCP, r.FCPMS),
					number(r.HasTTFB, r.TTFBMS),
					number(r.HasCLS, r.CLS),
					r.Errors,
				},
			)
		}
	case "rum-sessions", "rum-errors":
		var after, before int64
		after, before, err = historyRange(args, now)
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		if req.Method == "rum-sessions" {
			var sessions []store.RumSessionRecord
			sessions, err = h.deps.Sessions(ctx, args["site"], after, before)
			for _, s := range sessions {
				rows = append(
					rows,
					[]any{
						s.Site,
						s.SessionID,
						now - s.StartedAt,
						s.LastAt - s.StartedAt,
						s.Pageviews,
						s.Errors,
						s.Browser,
						s.Device,
						s.Country,
						s.Version,
						s.EntryPage,
						s.LastPage,
						h.redact.Apply(s.UserID),
						s.Frustrations,
					},
				)
			}
		} else {
			var groups []store.RumErrorAgg
			groups, err = h.deps.Errors(ctx, args["site"], args["fingerprint"], after, before)
			for _, g := range groups {
				rows = append(rows, []any{g.Site, g.Fingerprint, g.Type, h.redact.Apply(g.Message), g.CountWindow, detail(g.Details, g.SessionsAffected), now - g.FirstSeen, now - g.LastSeen, detail(g.Details, g.TopPage), detail(g.Details, strings.Join(g.Browsers, ",")), h.redact.Apply(g.SampleStack)})
			}
		}
	case "rum-session-events":
		if args["session_id"] == "" {
			return funcapi.ErrorResponse(400, "session_id is required")
		}
		session := args["session_id"]
		var events []store.RumSessionEventRecord
		events, err = h.deps.SessionEvents(ctx, args["site"], session)
		if err == nil && len(events) == 0 {
			return funcapi.ErrorResponse(404, "unknown session")
		}
		for _, e := range events {
			rows = append(rows, []any{e.TSUnixUS, e.Type, e.Page, h.redact.Apply(e.Text), e.TraceID})
		}
	}
	if err != nil {
		return functionError(err)
	}
	if rows == nil {
		rows = [][]any{}
	}
	response["data"] = rows
	return funcapi.RawResponse(response)
}
func detail(has bool, value any) any {
	if has {
		return value
	}
	return nil
}
func number(has bool, value float64) any {
	if has {
		return value
	}
	return nil
}
func boolean(v bool) int {
	if v {
		return 1
	}
	return 0
}
func historyRange(args map[string]string, now int64) (int64, int64, error) {
	var bounds [2]int64
	for i, key := range []string{"after", "before"} {
		if args[key] != "" {
			value, err := strconv.ParseInt(args[key], 10, 64)
			if err != nil {
				return 0, 0, fmt.Errorf("%s must be Unix seconds or a relative negative offset", key)
			}
			if value < 0 {
				value += now
			}
			bounds[i] = value
		}
	}
	if bounds[1] == 0 {
		bounds[1] = now
	}
	if bounds[0] == 0 {
		bounds[0] = bounds[1] - 900
	}
	if bounds[0] > bounds[1] {
		return 0, 0, errors.New("after must not exceed before")
	}
	return bounds[0], bounds[1], nil
}

func functionError(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(499, "request cancelled")
	}
	var invalid InvalidArgument
	if errors.As(err, &invalid) {
		return funcapi.ErrorResponse(400, "%v", invalid)
	}
	return funcapi.InternalErrorResponse("%v", err)
}
