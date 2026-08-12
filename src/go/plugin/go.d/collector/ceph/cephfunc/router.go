// SPDX-License-Identifier: GPL-3.0-or-later

package cephfunc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const maxFunctionTextLength = funcapi.DefaultMaxQueryLength

type router struct {
	deps   Deps
	config Config
}

var _ funcapi.MethodHandler = (*router)(nil)

func NewRouter(deps Deps, config Config) funcapi.MethodHandler {
	return &router{deps: deps, config: config}
}

func Methods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		{ID: MethodHealth, Name: "Ceph Health", UpdateEvery: 10, Help: "Detailed Ceph health checks and RCA messages."},
		{ID: MethodOSDs, Name: "Ceph OSDs", UpdateEvery: 30, Help: "Current Ceph OSD state, capacity, throughput, and latency."},
		{ID: MethodPools, Name: "Ceph Pools", UpdateEvery: 30, Help: "Ceph pool policy, placement, applications, and quotas."},
		{ID: MethodDaemons, Name: "Ceph Daemons", UpdateEvery: 30, Help: "Ceph daemon inventory reported by Dashboard."},
	}
}

func (r *router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if _, ok := r.methodConfig(method); !ok {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}

func (r *router) Handle(ctx context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	cfg, ok := r.methodConfig(method)
	if !ok {
		return funcapi.NotFoundResponse(method)
	}
	if cfg.Disabled {
		return funcapi.NotFoundResponse(method)
	}
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	switch method {
	case MethodHealth:
		return r.health(ctx, cfg.Limit)
	case MethodOSDs:
		return r.osds(ctx, cfg.Limit)
	case MethodPools:
		return r.pools(ctx, cfg.Limit)
	case MethodDaemons:
		return r.daemons(ctx, cfg.Limit)
	case MethodRGWMultisite:
		return r.rgwMultisite(ctx, cfg.Limit)
	case MethodRGWQuotas:
		return r.rgwQuotas(ctx, cfg.Limit)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (r *router) Cleanup(context.Context) {}

func (r *router) methodConfig(method string) (MethodConfig, bool) {
	switch method {
	case MethodHealth:
		return r.config.Health, true
	case MethodOSDs:
		return r.config.OSDs, true
	case MethodPools:
		return r.config.Pools, true
	case MethodDaemons:
		return r.config.Daemons, true
	case MethodRGWMultisite:
		return r.config.RGWMultisite, true
	case MethodRGWQuotas:
		return r.config.RGWQuotas, true
	default:
		return MethodConfig{}, false
	}
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
			detail, row.DetailTruncated || summaryTruncated || textTruncated, truncated,
		})
	}
	return tableResponse(healthColumns(), data, "severity", "Detailed Ceph health checks and RCA messages.", total, truncated)
}

func (r *router) osds(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.OSDs(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].ID < result.Rows[j].ID })
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row.UUID, row.ID, row.Name, row.Host, row.DeviceClass, row.Up, row.In, row.OperationalStatus,
			row.TotalBytes, row.UsedBytes, row.AvailableBytes, row.Utilization,
			row.ReadBytesPerSec, row.WriteBytesPerSec, row.ReadOpsPerSec, row.WriteOpsPerSec,
			row.CommitLatencyMS, row.ApplyLatencyMS, truncated,
		})
	}
	return tableResponse(osdColumns(), data, "id", "Current Ceph OSD state, capacity, throughput, and latency.", total, truncated)
}

func (r *router) pools(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.Pools(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].Name < result.Rows[j].Name })
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row.Name, row.Type, row.Size, row.MinSize, row.PGNum, row.PGPNum, row.PGAutoscaleMode,
			row.CrushRule, row.CrushRoot, row.FailureDomain, row.DeviceClass, row.Applications,
			row.ErasureProfile, row.QuotaMaxBytes, row.QuotaMaxObjects, row.Flags, truncated,
		})
	}
	return tableResponse(poolColumns(), data, "name", "Ceph pool policy, placement, applications, and quotas.", total, truncated)
}

func (r *router) daemons(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.Daemons(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool { return result.Rows[i].ID < result.Rows[j].ID })
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row.ID, row.Type, row.Name, row.Host, row.Status, row.Active, row.Version,
			row.Image, row.LastRefresh, row.Placement, truncated,
		})
	}
	return tableResponse(daemonColumns(), data, "type", "Ceph daemon inventory reported by Dashboard.", total, truncated)
}

func (r *router) rgwMultisite(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.RGWMultisite(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool {
		leftRank, rightRank := rgwKindRank(result.Rows[i].Kind), rgwKindRank(result.Rows[j].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if result.Rows[i].Name != result.Rows[j].Name {
			return result.Rows[i].Name < result.Rows[j].Name
		}
		return result.Rows[i].ID < result.Rows[j].ID
	})
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		detail, detailTruncated := truncateText(row.SyncDetail, maxFunctionTextLength)
		data = append(data, []any{
			row.ID, row.Kind, row.Name, row.Default, row.Realm, row.Zonegroup, row.Master,
			row.Endpoints, row.SyncStatus, detail, detailTruncated, row.ReleaseScope, truncated,
		})
	}
	return tableResponse(rgwMultisiteColumns(), data, "kind", "Opt-in RGW realm, zonegroup, zone, and best-effort sync diagnostics.", total, truncated)
}

func rgwKindRank(kind string) int {
	switch kind {
	case "realm":
		return 0
	case "zonegroup":
		return 1
	case "zone":
		return 2
	case "sync":
		return 3
	default:
		return 4
	}
}

func (r *router) rgwQuotas(ctx context.Context, limit int) *funcapi.FunctionResponse {
	result, err := r.deps.RGWQuotas(ctx, limit)
	if err != nil {
		return functionError(err)
	}
	sort.SliceStable(result.Rows, func(i, j int) bool {
		if result.Rows[i].Kind != result.Rows[j].Kind {
			return result.Rows[i].Kind < result.Rows[j].Kind
		}
		return result.Rows[i].ID < result.Rows[j].ID
	})
	rows, total, truncated := capRows(result.Rows, result.Total, limit)
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, []any{
			row.Key, row.ID, row.Kind, row.Status, row.Tenant, row.Account, row.Owner, row.UsedBytes, row.Objects,
			row.QuotaEnabled, row.QuotaMaxBytes, row.QuotaMaxObjects, row.Utilization,
			row.StatsFreshness, truncated,
		})
	}
	return tableResponse(rgwQuotaColumns(), data, "kind", "Opt-in quota and cached usage for explicitly configured RGW identities.", total, truncated)
}

func functionError(err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.DeadlineExceeded) {
		return funcapi.ErrorResponse(504, "Ceph Dashboard query timed out")
	}
	if errors.Is(err, context.Canceled) {
		return funcapi.ErrorResponse(499, "Ceph Dashboard query was canceled")
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

func capRows[T any](rows []T, total, limit int) ([]T, int, bool) {
	if total < len(rows) {
		total = len(rows)
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, total, total > len(rows)
}

func tableResponse(columns map[string]any, data [][]any, defaultSort, help string, total int, truncated bool) *funcapi.FunctionResponse {
	if truncated {
		help = fmt.Sprintf("%s Showing %d of %d rows; use collector configuration to change the bounded limit.", help, len(data), total)
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
		Name: name, Tooltip: tooltip, Type: funcapi.FieldTypeString, Visible: visible, UniqueKey: unique,
		Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount,
		Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformText,
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
		Name: name, Tooltip: tooltip, Type: funcapi.FieldTypeInteger, Units: units, Visible: visible,
		Sortable: true, Filter: funcapi.FieldFilterRange, Summary: funcapi.FieldSummarySum,
		Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber,
	}}
}

func floatCol(name, tooltip, units string, visible bool, decimals int) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name: name, Tooltip: tooltip, Type: funcapi.FieldTypeFloat, Units: units, Visible: visible,
		Sortable: true, Filter: funcapi.FieldFilterRange, Summary: funcapi.FieldSummaryMean,
		Visualization: funcapi.FieldVisualValue, Transform: funcapi.FieldTransformNumber, DecimalPoints: decimals,
	}}
}

func boolCol(name, tooltip string, visible bool) columnDef {
	return columnDef{funcapi.ColumnMeta{
		Name: name, Tooltip: tooltip, Type: funcapi.FieldTypeBoolean, Visible: visible,
		Sortable: true, Filter: funcapi.FieldFilterMultiselect, Summary: funcapi.FieldSummaryCount,
		Visualization: funcapi.FieldVisualPill, Transform: funcapi.FieldTransformNone,
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
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}

func osdColumns() map[string]any {
	return columns([]columnDef{
		textCol("uuid", "OSD UUID", false, true), intCol("id", "OSD numeric ID", "", true),
		textCol("name", "OSD name", true, false), textCol("host", "OSD host", true, false),
		textCol("device_class", "CRUSH device class", true, false), boolCol("up", "OSD is up", true),
		boolCol("in", "OSD is in", true), textCol("operational_status", "Orchestrator operational status", true, false),
		intCol("total_bytes", "Total OSD capacity", "bytes", false), intCol("used_bytes", "Used OSD capacity", "bytes", true),
		intCol("available_bytes", "Available OSD capacity", "bytes", true), floatCol("utilization", "OSD utilization", "%", true, 2),
		floatCol("read_bytes_per_sec", "Read throughput", "bytes/s", false, 2), floatCol("write_bytes_per_sec", "Write throughput", "bytes/s", false, 2),
		floatCol("read_ops_per_sec", "Read operations", "ops/s", false, 2), floatCol("write_ops_per_sec", "Write operations", "ops/s", false, 2),
		floatCol("commit_latency_ms", "Commit latency", "milliseconds", false, 3), floatCol("apply_latency_ms", "Apply latency", "milliseconds", false, 3),
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}

func poolColumns() map[string]any {
	return columns([]columnDef{
		textCol("name", "Pool name", true, true), textCol("type", "Pool type", true, false),
		intCol("size", "Replica or EC shard count", "copies", true), intCol("min_size", "Minimum available copies", "copies", true),
		intCol("pg_num", "Placement groups", "PGs", true), intCol("pgp_num", "Placement groups for placement", "PGs", false),
		textCol("pg_autoscale_mode", "PG autoscaler mode", true, false), textCol("crush_rule", "CRUSH rule", true, false),
		textCol("crush_root", "CRUSH root", true, false), textCol("failure_domain", "CRUSH failure domain", true, false),
		textCol("device_class", "CRUSH device class", true, false), textCol("applications", "Enabled pool applications", true, false),
		textCol("erasure_profile", "Erasure-code profile", false, false), intCol("quota_max_bytes", "Pool byte quota", "bytes", false),
		intCol("quota_max_objects", "Pool object quota", "objects", false), textCol("flags", "Pool flags", false, false),
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}

func daemonColumns() map[string]any {
	return columns([]columnDef{
		textCol("id", "Daemon row identifier", false, true), textCol("type", "Daemon type", true, false),
		textCol("name", "Daemon name", true, false), textCol("host", "Daemon host", true, false),
		textCol("status", "Daemon status", true, false), boolCol("active", "Daemon active state when Dashboard reports it", true),
		textCol("version", "Ceph version", true, false), textCol("image", "Container image", false, false),
		textCol("last_refresh", "Last inventory refresh", false, false), textCol("placement", "Orchestrator placement", false, false),
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}

func rgwMultisiteColumns() map[string]any {
	return columns([]columnDef{
		textCol("id", "RGW multisite row identifier", false, true), textCol("kind", "Realm, zonegroup, zone, or sync row", true, false),
		textCol("name", "Object name", true, false), boolCol("default", "Default object when Ceph reports the relationship", true),
		textCol("realm", "Realm", true, false), textCol("zonegroup", "Zonegroup", true, false),
		boolCol("master", "Zonegroup or zone master relationship when Ceph reports it", true), wrappedTextCol("endpoints", "Configured endpoints", false),
		textCol("sync_status", "Best-effort sync status", true, false), wrappedTextCol("sync_detail", "Best-effort sync detail", false),
		boolCol("sync_detail_truncated", "Sync detail was truncated", false), textCol("release_scope", "Release/API capability note", false, false),
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}

func rgwQuotaColumns() map[string]any {
	return columns([]columnDef{
		textCol("key", "Kind-qualified RGW row identifier", false, true), textCol("id", "Configured RGW identity", true, false),
		textCol("kind", "User, bucket, or account", true, false),
		textCol("status", "Lookup status", true, false), textCol("tenant", "RGW tenant", true, false), textCol("account", "RGW account", true, false),
		textCol("owner", "Bucket or account owner", true, false), intCol("used_bytes", "Cached quota usage", "bytes", true),
		intCol("objects", "Cached object count", "objects", true), boolCol("quota_enabled", "Quota is enabled", true),
		intCol("quota_max_bytes", "Quota byte limit", "bytes", true), intCol("quota_max_objects", "Quota object limit", "objects", true),
		floatCol("utilization", "Usage against enabled byte quota", "%", true, 2), textCol("stats_freshness", "Quota usage freshness semantics", false, false),
		boolCol("truncated", "Result was truncated by the configured row limit", false),
	})
}
