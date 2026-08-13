// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	ParamLimit            = "limit"
	healthResultLimit     = 500
	functionHealthTimeout = 5 * time.Second
	functionOSDTimeout    = 5 * time.Second
	functionPoolTimeout   = 8 * time.Second
	functionDaemonTimeout = 5 * time.Second
	maxFunctionTextLength = funcapi.DefaultMaxQueryLength
)

type router struct {
	deps Deps
}

var _ funcapi.MethodHandler = (*router)(nil)

func NewRouter(deps Deps) funcapi.MethodHandler {
	return &router{
		deps: deps,
	}
}

func Methods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		{ID: MethodHealth, Name: "Ceph Health", UpdateEvery: 10, Help: "Detailed Ceph health checks and RCA messages."},
		{
			ID:             MethodOSDs,
			Name:           "Ceph OSDs",
			UpdateEvery:    30,
			Help:           "Current Ceph OSD state, capacity, throughput, and latency.",
			RequiredParams: inventoryParams(),
		},
		{
			ID:             MethodPools,
			Name:           "Ceph Pools",
			UpdateEvery:    30,
			Help:           "Ceph pool policy, placement, applications, and quotas.",
			RequiredParams: inventoryParams(),
		},
		{
			ID:             MethodDaemons,
			Name:           "Ceph Daemons",
			UpdateEvery:    30,
			Help:           "Ceph daemon inventory reported by Dashboard.",
			RequiredParams: inventoryParams(),
		},
	}
}

func (r *router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case MethodHealth, MethodOSDs, MethodPools, MethodDaemons:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func (r *router) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	timeout, ok := methodTimeout(method)
	if !ok {
		return funcapi.NotFoundResponse(method)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch method {
	case MethodHealth:
		return r.health(ctx, healthResultLimit)
	case MethodOSDs:
		limit, err := inventoryLimit(params)
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		return r.osds(ctx, limit)
	case MethodPools:
		limit, err := inventoryLimit(params)
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		return r.pools(ctx, limit)
	case MethodDaemons:
		limit, err := inventoryLimit(params)
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		return r.daemons(ctx, limit)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (r *router) Cleanup(context.Context) {}

func methodTimeout(method string) (time.Duration, bool) {
	switch method {
	case MethodHealth:
		return functionHealthTimeout, true
	case MethodOSDs:
		return functionOSDTimeout, true
	case MethodPools:
		return functionPoolTimeout, true
	case MethodDaemons:
		return functionDaemonTimeout, true
	default:
		return 0, false
	}
}

func inventoryParams() []funcapi.ParamConfig {
	return []funcapi.ParamConfig{{
		ID: ParamLimit, Name: "Maximum rows", Help: "Return the complete inventory only when it fits within this limit.",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: "100", Name: "100 rows"},
			{ID: strconv.Itoa(DefaultInventoryLimit), Name: "500 rows", Default: true},
			{ID: "1000", Name: "1,000 rows"},
			{ID: "2500", Name: "2,500 rows"},
			{ID: strconv.Itoa(MaxInventoryLimit), Name: "5,000 rows"},
		},
	}}
}

func inventoryLimit(params funcapi.ResolvedParams) (int, error) {
	value := params.GetOne(ParamLimit)
	if value == "" {
		return DefaultInventoryLimit, nil
	}
	for _, option := range inventoryParams()[0].Options {
		if value == option.ID {
			limit, _ := strconv.Atoi(value)
			return limit, nil
		}
	}
	return 0, fmt.Errorf("unsupported inventory limit %q", value)
}

func (r *router) health(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.Health(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool {
		if severityRank(result.Rows[i].Severity) != severityRank(result.Rows[j].Severity) {
			return severityRank(result.Rows[i].Severity) > severityRank(result.Rows[j].Severity)
		}
		return result.Rows[i].ID < result.Rows[j].ID
	})
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		summary, summaryTruncated := truncateText(row.Summary, maxFunctionTextLength)
		detail, textTruncated := truncateText(row.Detail, maxFunctionTextLength)
		data = append(data, []any{
			row.ID, row.Code, row.Severity, row.Muted, summary, row.Count,
			detail, summaryTruncated || textTruncated, truncated,
		})
	}
	return tableResponse(
		healthColumns(),
		data,
		"severity",
		"Detailed Ceph health checks and RCA messages.",
		total,
		truncated,
	)
}

func (r *router) osds(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.OSDs(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	if err := validateCompleteInventory("OSD", len(result.Rows), result.Total, limit); err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].ID < result.Rows[j].ID })
	data := make([][]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		data = append(data, []any{
			row.UUID, row.ID, row.Name, row.Host, row.DeviceClass, row.Up, row.In, row.OperationalStatus,
			row.TotalBytes, row.UsedBytes, row.AvailableBytes, row.Utilization,
			row.ReadBytesPerSec, row.WriteBytesPerSec, row.ReadOpsPerSec, row.WriteOpsPerSec,
			row.CommitLatencyMS, row.ApplyLatencyMS,
		})
	}
	return tableResponse(
		osdColumns(),
		data,
		"id",
		"Current Ceph OSD state, capacity, throughput, and latency.",
		result.Total,
		false,
	)
}

func (r *router) pools(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.Pools(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	if err := validateCompleteInventory("pool", len(result.Rows), result.Total, limit); err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].Name < result.Rows[j].Name })
	data := make([][]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		data = append(data, []any{
			row.Name, row.Type, row.Size, row.MinSize, row.PGNum, row.PGPNum, row.PGAutoscaleMode,
			row.CrushRule, row.CrushRoot, row.FailureDomain, row.DeviceClass, row.Applications,
			row.ErasureProfile, row.QuotaMaxBytes, row.QuotaMaxObjects, row.Flags,
		})
	}
	return tableResponse(
		poolColumns(),
		data,
		"name",
		"Ceph pool policy, placement, applications, and quotas.",
		result.Total,
		false,
	)
}

func (r *router) daemons(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.Daemons(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	if err := validateCompleteInventory("daemon", len(result.Rows), result.Total, limit); err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].ID < result.Rows[j].ID })
	data := make([][]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		data = append(data, []any{
			row.ID, row.Type, row.Name, row.Host, row.Status, row.Active, row.Version,
			row.Image, row.LastRefresh, row.Placement,
		})
	}
	return tableResponse(
		daemonColumns(),
		data,
		"type",
		"Ceph daemon inventory reported by Dashboard.",
		result.Total,
		false,
	)
}

func functionError(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(504, "Ceph Dashboard query timed out")
	}
	if errors.Is(err, context.Canceled) {
		return funcapi.ErrorResponse(499, "Ceph Dashboard query was canceled")
	}
	var limitErr *InventoryLimitError
	if errors.As(err, &limitErr) {
		return funcapi.ErrorResponse(422, "%s", limitErr.Error())
	}
	var incompleteErr *IncompleteInventoryError
	if errors.As(err, &incompleteErr) {
		return funcapi.ErrorResponse(422, "%s", incompleteErr.Error())
	}
	var sourceErr *SourceError
	if errors.As(err, &sourceErr) {
		status := sourceErr.Status
		if status < 400 || status > 599 {
			status = 500
		}
		return funcapi.ErrorResponse(status, "%s", sourceErr.Error())
	}
	return funcapi.ErrorResponse(500, "Ceph Dashboard query failed")
}

func validateCompleteInventory(resource string, rows, total, limit int) error {
	if total > MaxInventoryLimit {
		return &InventoryLimitError{
			Resource: resource,
			Total:    total,
			Limit:    MaxInventoryLimit,
			Hard:     true,
		}
	}
	if total > limit {
		return &InventoryLimitError{
			Resource: resource,
			Total:    total,
			Limit:    limit,
		}
	}
	if total < 0 {
		return fmt.Errorf("Ceph %s inventory returned a negative total", resource)
	}
	if rows != total {
		return &IncompleteInventoryError{
			Resource: resource,
			Rows:     rows,
			Total:    total,
		}
	}
	return nil
}

func capRows[T any](rows []T, total, limit int) ([]T, int, bool) {
	if total < len(rows) {
		total = len(rows)
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, total, total > len(rows)
}

func tableResponse(
	columns map[string]any,
	data [][]any,
	defaultSort, help string,
	total int,
	truncated bool,
) *funcapi.FunctionResponse {
	if truncated {
		help = fmt.Sprintf("%s Showing the %d most severe of %d rows.", help, len(data), total)
	}
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              help,
		Columns:           columns,
		Data:              data,
		DefaultSortColumn: defaultSort,
	}
}

func truncateText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit], true
}

func severityRank(severity string) int {
	switch severity {
	case "HEALTH_ERR":
		return 3
	case "HEALTH_WARN":
		return 2
	case "HEALTH_OK":
		return 1
	default:
		return 0
	}
}

type columnDef struct{ funcapi.ColumnMeta }

func columns(defs []columnDef) map[string]any {
	return funcapi.Columns(defs, func(def columnDef) funcapi.ColumnMeta { return def.ColumnMeta }).BuildColumns()
}

func textCol(name, tooltip string, visible, unique bool) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name:          name,
		Tooltip:       tooltip,
		Type:          funcapi.FieldTypeString,
		Visible:       visible,
		UniqueKey:     unique,
		Sortable:      true,
		Filter:        funcapi.FieldFilterMultiselect,
		Summary:       funcapi.FieldSummaryCount,
		Visualization: funcapi.FieldVisualValue,
		Transform:     funcapi.FieldTransformText,
	}}
}

func wrappedTextCol(name, tooltip string, visible bool) columnDef {
	col := textCol(name, tooltip, visible, false)
	col.Wrap = true
	col.Filter = funcapi.FieldFilterNone
	return col
}

func intCol(name, tooltip, units string, visible bool) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name:          name,
		Tooltip:       tooltip,
		Type:          funcapi.FieldTypeInteger,
		Units:         units,
		Visible:       visible,
		Sortable:      true,
		Filter:        funcapi.FieldFilterRange,
		Summary:       funcapi.FieldSummarySum,
		Visualization: funcapi.FieldVisualValue,
		Transform:     funcapi.FieldTransformNumber,
	}}
}

func floatCol(name, tooltip, units string, visible bool, decimals int) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name:          name,
		Tooltip:       tooltip,
		Type:          funcapi.FieldTypeFloat,
		Units:         units,
		Visible:       visible,
		Sortable:      true,
		Filter:        funcapi.FieldFilterRange,
		Summary:       funcapi.FieldSummaryMean,
		Visualization: funcapi.FieldVisualValue,
		Transform:     funcapi.FieldTransformNumber,
		DecimalPoints: decimals,
	}}
}

func boolCol(name, tooltip string, visible bool) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name:          name,
		Tooltip:       tooltip,
		Type:          funcapi.FieldTypeBoolean,
		Visible:       visible,
		Sortable:      true,
		Filter:        funcapi.FieldFilterMultiselect,
		Summary:       funcapi.FieldSummaryCount,
		Visualization: funcapi.FieldVisualPill,
		Transform:     funcapi.FieldTransformNone,
	}}
}

func healthColumns() map[string]any {
	return columns([]columnDef{
		textCol("id", "Health detail row identifier", false, true),
		textCol("code", "Ceph health-check code", true, false),
		textCol("severity", "Ceph health severity", true, false),
		boolCol("muted", "Whether Ceph muted the health check", true),
		wrappedTextCol("summary", "Health-check summary", true),
		intCol("count", "Affected item count", "items", true),
		wrappedTextCol("detail", "Detailed health-check message", true),
		boolCol("detail_truncated", "Summary or detailed message was truncated", false),
		boolCol("truncated", "Result contains only the most severe rows allowed by the internal health limit", false),
	})
}

func osdColumns() map[string]any {
	return columns([]columnDef{
		textCol("uuid", "OSD UUID", false, true), intCol("id", "OSD numeric ID", "", true),
		textCol("name", "OSD name", true, false), textCol("host", "OSD host", true, false),
		textCol("device_class", "CRUSH device class", true, false), boolCol("up", "OSD is up", true),
		boolCol("in", "OSD is in", true), textCol("operational_status", "Orchestrator operational status", true, false),
		intCol(
			"total_bytes",
			"Total OSD capacity",
			"bytes",
			false,
		), intCol("used_bytes", "Used OSD capacity", "bytes", true),
		intCol(
			"available_bytes",
			"Available OSD capacity",
			"bytes",
			true,
		), floatCol("utilization", "OSD utilization", "%", true, 2),
		floatCol(
			"read_bytes_per_sec",
			"Read throughput",
			"bytes/s",
			false,
			2,
		), floatCol("write_bytes_per_sec", "Write throughput", "bytes/s", false, 2),
		floatCol(
			"read_ops_per_sec",
			"Read operations",
			"ops/s",
			false,
			2,
		), floatCol("write_ops_per_sec", "Write operations", "ops/s", false, 2),
		floatCol(
			"commit_latency_ms",
			"Commit latency",
			"milliseconds",
			false,
			3,
		), floatCol("apply_latency_ms", "Apply latency", "milliseconds", false, 3),
	})
}

func poolColumns() map[string]any {
	return columns([]columnDef{
		textCol("name", "Pool name", true, true), textCol("type", "Pool type", true, false),
		intCol(
			"size",
			"Replica or EC shard count",
			"copies",
			true,
		), intCol("min_size", "Minimum available copies", "copies", true),
		intCol(
			"pg_num",
			"Placement groups",
			"PGs",
			true,
		), intCol("pgp_num", "Placement groups for placement", "PGs", false),
		textCol(
			"pg_autoscale_mode",
			"PG autoscaler mode",
			true,
			false,
		), textCol("crush_rule", "CRUSH rule", true, false),
		textCol(
			"crush_root",
			"CRUSH root",
			true,
			false,
		), textCol("failure_domain", "CRUSH failure domain", true, false),
		textCol(
			"device_class",
			"CRUSH device class",
			true,
			false,
		), textCol("applications", "Enabled pool applications", true, false),
		textCol(
			"erasure_profile",
			"Erasure-code profile",
			false,
			false,
		), intCol("quota_max_bytes", "Pool byte quota", "bytes", false),
		intCol(
			"quota_max_objects",
			"Pool object quota",
			"objects",
			false,
		), textCol("flags", "Pool flags", false, false),
	})
}

func daemonColumns() map[string]any {
	return columns([]columnDef{
		textCol("id", "Daemon row identifier", false, true), textCol("type", "Daemon type", true, false),
		textCol("name", "Daemon name", true, false), textCol("host", "Daemon host", true, false),
		textCol(
			"status",
			"Daemon status",
			true,
			false,
		), boolCol("active", "Daemon active state when Dashboard reports it", true),
		textCol("version", "Ceph version", true, false), textCol("image", "Container image", false, false),
		textCol(
			"last_refresh",
			"Last inventory refresh",
			false,
			false,
		), textCol("placement", "Orchestrator placement", false, false),
	})
}
