// SPDX-License-Identifier: GPL-3.0-or-later

// Package historyfunc presents process-owned history storage status.
package historyfunc

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
)

// Deps exposes storage facts without owning the process store.
type Deps interface {
	Status(context.Context) (journalstore.Status, error)
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
	Help: "Status of the history storage shared by RUM and synthetic monitoring: retention policy, journal " +
		"size (including preallocated space) and file count, cleanup times, and any inventory, cleanup, writer " +
		"or configuration reload error. It cannot show whether history is complete or how far back it reaches.",
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
	if _, r := rawfunc.Admit(req, nil, nil); r != nil {
		return r
	}
	// Metadata remains available without accessing the store or reload state.
	if req.Info {
		return &funcapi.FunctionResponse{
			Status: 200,
			Help:   method.Help,
		}
	}
	if r := rawfunc.Cancelled(ctx.Err()); r != nil {
		return r
	}
	status, err := h.deps.Status(ctx)
	if err != nil {
		if r := rawfunc.Cancelled(err); r != nil {
			return r
		}
		return funcapi.UnavailableResponse(err.Error())
	}
	response := method.Table()
	response["data"] = [][]any{statusColumns.Row(statusRow{
		Status:      status,
		policyError: h.policyError(),
	})}
	return funcapi.RawResponse(response)
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

// statusRow is the store status with the retention worker's reload error.
type statusRow struct {
	journalstore.Status
	policyError string
}

// column declares a visible column. Timestamps are Unix microseconds.
func column(
	name, tooltip string,
	kind funcapi.FieldType,
	units string,
	value func(statusRow) any,
) rawfunc.Column[statusRow] {
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
