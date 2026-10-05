// SPDX-License-Identifier: GPL-3.0-or-later

// Package functions owns process-level RUM Functions behind admitted data snapshots.
package functions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

// Deps exposes copied query results, independent of collectors and persistence.
type Deps interface {
	Receiver() query.Receiver
	Sites(context.Context) ([]query.Site, error)
	Pages(context.Context, string) ([]query.Page, error)
	Live(context.Context, string, string, int) ([]query.LiveEvent, string, error)
	Sessions(context.Context, string, string, int64, int64) ([]query.Session, error)
	SessionEvents(context.Context, string, string) ([]query.SessionEvent, error)
	Errors(context.Context, string, string, int64, int64) ([]query.ErrorGroup, error)
}

type Handler struct {
	deps   Deps
	redact *redact.Redactor
}

func New(deps Deps) *Handler {
	return &Handler{
		deps:   deps,
		redact: redact.NewRedactor(),
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
		problemBiased := false
		for _, site := range sites {
			if args["site"] != "" && args["site"] != site.Name {
				continue
			}
			notes = append(notes, samplingNote(site))
			sampling := site.Sampling
			if sampling.InvestigateRate < 1 && (sampling.KeepErrors || sampling.KeepPoorVitals) {
				problemBiased = true
			}
		}
		if len(notes) > 0 {
			help := spec.help + ". Current sampling policy: " + strings.Join(notes, "; ") +
				". Historical evidence may reflect earlier policies, bounded context, retention and delivery loss. " +
				"These rows do not estimate all visitors or errors."
			if problemBiased {
				help += " Problem overrides bias retained detail toward failures."
			}
			response["help"] = help
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
		var receiver query.Receiver
		rows, receiver, err = h.sitesRows(ctx, now)
		response["collector"] = map[string]any{"public_url": receiver.PublicURL, "ingress_available": receiver.Serving}
	case "rum-pages":
		rows, err = h.pagesRows(ctx, args["site"])
	case "rum-live":
		var next string
		rows, next, err = h.liveRows(ctx, args["site"], args["after"])
		response["next"] = next
	case "rum-sessions", "rum-errors":
		var after, before int64
		after, before, err = historyRange(args, now)
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		if req.Method == "rum-sessions" {
			rows, err = h.sessionsRows(ctx, args["site"], args["user_id"], after, before, now)
		} else {
			rows, err = h.errorsRows(ctx, args["site"], args["fingerprint"], after, before, now)
		}
	case "rum-session-events":
		if args["session_id"] == "" {
			return funcapi.ErrorResponse(400, "session_id is required")
		}
		rows, err = h.sessionEventsRows(ctx, args["site"], args["session_id"])
		if err == nil && len(rows) == 0 {
			return funcapi.ErrorResponse(404, "unknown session")
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

func functionError(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(499, "request cancelled")
	}
	var invalid query.InvalidArgument
	if errors.As(err, &invalid) {
		return funcapi.ErrorResponse(400, "%v", invalid)
	}
	return funcapi.InternalErrorResponse("%v", err)
}
