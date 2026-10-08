// SPDX-License-Identifier: GPL-3.0-or-later

// Package historyfunc presents process-owned history storage status.
package historyfunc

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
)

// Deps exposes storage facts without owning the process store.
type Deps interface {
	Status(context.Context) (journal.Status, error)
}

type Handler struct {
	deps        Deps
	policyError func() string
}

func New(deps Deps, policyError func() string) *Handler {
	return &Handler{
		deps:        deps,
		policyError: policyError,
	}
}
func (h *Handler) Cleanup(context.Context) {}
func (h *Handler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}
func (h *Handler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "raw history status request required")
}

const method = "dem-history-status"
const help = "Current shared DEM history policy, journal file bytes including preallocation, and maintenance health. " +
	"These facts do not establish complete history or an exact retained time window."

func Declarations() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{
		ID:           method,
		FunctionName: method,
		Name:         "DEM History Status",
		UpdateEvery:  60,
		Help:         help,
		Tags:         "dem",
		RawRequest:   true,
		ManagedInfo:  true,
	}}
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if req.Method != method {
		return funcapi.NotFoundResponse(req.Method)
	}
	if len(req.Payload) > 0 {
		return funcapi.ErrorResponse(400, "this read-only Function does not accept a payload")
	}
	for _, arg := range req.Args {
		if req.Info && arg == "info" {
			continue
		}
		return funcapi.ErrorResponse(400, "history status does not accept arguments")
	}
	response := &funcapi.FunctionResponse{
		Status:            200,
		Help:              help,
		Columns:           columns(),
		DefaultSortColumn: "observed_us",
	}
	// Metadata remains available without accessing the store or reload state.
	if req.Info {
		return response
	}
	mask, err := strconv.ParseUint(req.Permissions, 0, 64)
	if err != nil || mask&0x1b != 0x1b {
		return funcapi.ErrorResponse(403, "member permissions are required")
	}
	if err := ctx.Err(); err != nil {
		return statusError(err)
	}
	if h.deps == nil {
		return funcapi.UnavailableResponse("history storage is unavailable")
	}
	status, err := h.deps.Status(ctx)
	if err != nil {
		return statusError(err)
	}
	var days, maxBytes, fileBytes, fileCount any
	if status.Policy != nil {
		days, maxBytes = status.Policy.Days, status.Policy.MaxBytes
	}
	if status.Inventory != nil {
		fileBytes, fileCount = status.Inventory.Bytes, status.Inventory.Files
	}
	var policyError string
	if h.policyError != nil {
		policyError = h.policyError()
	}
	response.Data = [][]any{{
		days, maxBytes, fileBytes, fileCount,
		timestamp(status.ObservedAt), nullable(status.InventoryError),
		timestamp(status.Cleanup.AttemptedAt), timestamp(status.Cleanup.LastSuccessfulAt),
		nullable(status.Cleanup.Error), nullable(status.WriterError), nullable(policyError),
	}}
	return response
}
func timestamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMicro()
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func statusError(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.Canceled) {
		return funcapi.ErrorResponse(499, "request cancelled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(504, "request timed out")
	}
	return funcapi.UnavailableResponse(err.Error())
}

func columns() map[string]any {
	specs := []struct {
		id, title, units string
		kind             funcapi.FieldType
	}{
		{"policy_days", "Effective age policy", "days", funcapi.FieldTypeInteger},
		{"policy_max_bytes", "Effective file byte target", "bytes", funcapi.FieldTypeInteger},
		{"file_bytes", "Observed file bytes including preallocation", "bytes", funcapi.FieldTypeInteger},
		{"file_count", "Observed journal files", "files", funcapi.FieldTypeInteger},
		{"observed_us", "Inventory observation time", "", funcapi.FieldTypeTimestamp},
		{"inventory_error", "Inventory error", "", funcapi.FieldTypeString},
		{"cleanup_attempt_us", "Latest cleanup attempt", "", funcapi.FieldTypeTimestamp},
		{"cleanup_success_us", "Last successful cleanup", "", funcapi.FieldTypeTimestamp},
		{"cleanup_error", "Latest cleanup error", "", funcapi.FieldTypeString},
		{"writer_error", "Writer error", "", funcapi.FieldTypeString},
		{"policy_error", "Configuration reload error", "", funcapi.FieldTypeString},
	}
	out := make(map[string]any, len(specs))
	for i, s := range specs {
		c := funcapi.Column{
			Index:         i,
			Name:          s.title,
			Type:          s.kind,
			Units:         s.units,
			Visible:       true,
			Visualization: funcapi.FieldVisualValue,
		}
		if s.kind == funcapi.FieldTypeTimestamp {
			c.ValueOptions.Transform = funcapi.FieldTransformDatetimeUsec
		}
		out[s.id] = c.BuildColumn()
	}
	return out
}
