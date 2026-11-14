// SPDX-License-Identifier: GPL-3.0-or-later

// Package historyfunc presents process-owned history storage status.
package historyfunc

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
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

var method = rawfunc.Method{
	ID:    "dem-history-status",
	Title: "DEM History Status",
	Help: "Current shared DEM history policy, journal file bytes including preallocation, and maintenance health. " +
		"These facts do not establish complete history or an exact retained time window.",
	Sort:        "observed_us",
	UpdateEvery: 60,
	Columns:     statusColumns.Build(),
}

func Declarations() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{method.Declaration("dem")}
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if req.Method != method.ID {
		return funcapi.NotFoundResponse(req.Method)
	}
	if r := rawfunc.NoPayload(req); r != nil {
		return r
	}
	if _, err := rawfunc.ParseArgs(req, nil); err != nil {
		return funcapi.ErrorResponse(400, "history status does not accept arguments")
	}
	response := &funcapi.FunctionResponse{
		Status:            200,
		Help:              method.Help,
		Columns:           method.Columns,
		DefaultSortColumn: method.Sort,
	}
	// Metadata remains available without accessing the store or reload state.
	if req.Info {
		return response
	}
	if r := rawfunc.RequireMember(req); r != nil {
		return r
	}
	if err := ctx.Err(); err != nil {
		return statusError(err)
	}
	status, err := h.deps.Status(ctx)
	if err != nil {
		return statusError(err)
	}
	response.Data = [][]any{statusColumns.Row(statusRow{Status: status, policyError: h.policyError()})}
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
	switch {
	case errors.Is(err, context.Canceled):
		return rawfunc.Cancelled(err)
	case errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(504, "request timed out")
	default:
		return funcapi.UnavailableResponse(err.Error())
	}
}

// statusRow is the store status with the retention worker's reload error.
type statusRow struct {
	journal.Status
	policyError string
}

// column declares a visible column. Timestamps are Unix microseconds.
func column(name, tooltip string, kind funcapi.FieldType, units string, value func(statusRow) any) rawfunc.Column[statusRow] {
	col := rawfunc.Column[statusRow]{
		Name:    name,
		Tooltip: tooltip,
		Type:    kind,
		Units:   units,
		Visible: true,
		Value:   value,
	}
	if kind == funcapi.FieldTypeTimestamp {
		col.Transform = funcapi.FieldTransformDatetimeUsec
	}
	return col
}

var statusColumns = rawfunc.Columns[statusRow]{
	column("policy_days", "Effective age policy", funcapi.FieldTypeInteger, "days", func(s statusRow) any {
		if s.Policy == nil {
			return nil
		}
		return s.Policy.Days
	}),
	column("policy_max_bytes", "Effective file byte target", funcapi.FieldTypeInteger, "bytes", func(s statusRow) any {
		if s.Policy == nil {
			return nil
		}
		return s.Policy.MaxBytes
	}),
	column("file_bytes", "Observed file bytes including preallocation", funcapi.FieldTypeInteger, "bytes",
		func(s statusRow) any {
			if s.Inventory == nil {
				return nil
			}
			return s.Inventory.Bytes
		}),
	column("file_count", "Observed journal files", funcapi.FieldTypeInteger, "files", func(s statusRow) any {
		if s.Inventory == nil {
			return nil
		}
		return s.Inventory.Files
	}),
	column("observed_us", "Inventory observation time", funcapi.FieldTypeTimestamp, "", func(s statusRow) any {
		return timestamp(s.ObservedAt)
	}),
	column("inventory_error", "Inventory error", funcapi.FieldTypeString, "", func(s statusRow) any {
		return nullable(s.InventoryError)
	}),
	column("cleanup_attempt_us", "Latest cleanup attempt", funcapi.FieldTypeTimestamp, "", func(s statusRow) any {
		return timestamp(s.Cleanup.AttemptedAt)
	}),
	column("cleanup_success_us", "Last successful cleanup", funcapi.FieldTypeTimestamp, "", func(s statusRow) any {
		return timestamp(s.Cleanup.LastSuccessfulAt)
	}),
	column("cleanup_error", "Latest cleanup error", funcapi.FieldTypeString, "", func(s statusRow) any {
		return nullable(s.Cleanup.Error)
	}),
	column("writer_error", "Writer error", funcapi.FieldTypeString, "", func(s statusRow) any {
		return nullable(s.WriterError)
	}),
	column("policy_error", "Configuration reload error", funcapi.FieldTypeString, "", func(s statusRow) any {
		return nullable(s.policyError)
	}),
}
