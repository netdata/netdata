// SPDX-License-Identifier: GPL-3.0-or-later

// Package rumfunc owns process-level RUM Functions behind admitted data snapshots.
package rumfunc

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
)

// Deps exposes copied query results, independent of collectors and persistence.
type Deps interface {
	Receiver() query.Receiver
	Sites(context.Context, string) ([]query.Site, error)
	Pages(context.Context, string, string) ([]query.Page, error)
	Live(context.Context, string, string, int, string) ([]query.LiveEvent, string, error)
	Sessions(context.Context, string, string, int64, int64, int, string) ([]query.Session, error)
	SessionEvents(context.Context, string, string, string) ([]query.SessionEvent, error)
	Errors(context.Context, string, string, int64, int64, string) ([]query.ErrorGroup, error)
}

type Handler struct {
	deps            Deps
	lookupVnodeGUID func(string) (string, bool)
}

func New(deps Deps, lookupVnodeGUID func(string) (string, bool)) *Handler {
	return &Handler{
		deps:            deps,
		lookupVnodeGUID: lookupVnodeGUID,
	}
}

func (h *Handler) Cleanup(context.Context) {}

func (h *Handler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (h *Handler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "raw RUM request required")
}

type method struct {
	rawfunc.Method
	// samplingHelp adds the selected sites' current sampling policy to the help.
	samplingHelp bool
	// rows reads the method's rows and may add method-specific response fields.
	rows func(h *Handler, ctx context.Context, args map[string]string, response map[string]any) ([][]any, error)
}

var methods = []method{sitesMethod, pagesMethod, liveMethod, sessionsMethod, errorsMethod, sessionEventsMethod}

func Declarations() []funcapi.FunctionConfig {
	out := make([]funcapi.FunctionConfig, 0, len(methods))
	for _, m := range methods {
		out = append(out, m.Declaration("rum"))
	}
	return out
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	i := slices.IndexFunc(methods, func(m method) bool { return m.ID == req.Method })
	if i < 0 {
		return funcapi.NotFoundResponse(req.Method)
	}
	spec := methods[i]
	// Info is metadata; data needs member access before any argument or source is read.
	if !req.Info {
		if r := rawfunc.RequireMember(req); r != nil {
			return r
		}
	}
	args, err := rawfunc.ParseArgs(req, spec.Params)
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	if err := rawfunc.ResolveVnode(args, h.lookupVnodeGUID); err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	response := spec.Table()
	if spec.samplingHelp {
		help, err := h.samplingHelp(ctx, spec.Help, args)
		if err != nil {
			return functionError(err)
		}
		response["help"] = help
	}
	if req.Info {
		return &funcapi.FunctionResponse{
			Status: 200,
			Help:   response["help"].(string),
		}
	}
	if r := rawfunc.Cancelled(ctx.Err()); r != nil {
		return r
	}
	rows, err := spec.rows(h, ctx, args, response)
	if err != nil {
		return functionError(err)
	}
	// The table already holds an empty data list for methods without rows.
	if rows != nil {
		response["data"] = rows
	}
	return funcapi.RawResponse(response)
}

// credentialPatterns guards evidence text with the shared credential patterns.
// Query redacts active sites with their site redactor; retired-site history is
// not redacted there, and ingestion redacts before it strips control characters.
var credentialPatterns = redact.NewRedactor()

func detail(has bool, value any) any {
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

// rowID prefixes the site's byte length so arbitrary site-local values cannot collide.
func rowID(site, value string) string { return strconv.Itoa(len(site)) + ":" + site + value }

// requestError is a client error with its own status.
type requestError struct {
	status  int
	message string
}

func (e requestError) Error() string { return e.message }

func badRequest(err error) requestError { return requestError{status: 400, message: err.Error()} }

func functionError(err error) *funcapi.FunctionResponse {
	if r := rawfunc.Cancelled(err); r != nil {
		return r
	}
	var request requestError
	if errors.As(err, &request) {
		return funcapi.ErrorResponse(request.status, "%s", request.message)
	}
	var invalid query.InvalidArgument
	if errors.As(err, &invalid) {
		return funcapi.ErrorResponse(400, "%v", invalid)
	}
	return funcapi.InternalErrorResponse("%v", err)
}
