// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ceph/cephfunc"
)

const (
	urlPathAPICrushRule = "/api/crush_rule"
	urlPathAPIDaemon    = "/api/daemon"
	hdrAcceptVersionV2  = "application/vnd.ceph.api.v2.0+json"
)

type funcDepsAdapter struct {
	collector *Collector
}

func (d funcDepsAdapter) revalidateClusterIdentity(ctx context.Context) error {
	_, err := d.collector.probeClusterIdentity(ctx)
	return err
}

type apiHealthCheck struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Muted    bool   `json:"muted"`
	Summary  struct {
		Message string `json:"message"`
		Count   int64  `json:"count"`
	} `json:"summary"`
	Detail []struct {
		Message string `json:"message"`
	} `json:"detail"`
}

type apiHealthChecks []apiHealthCheck

func (c *apiHealthChecks) UnmarshalJSON(data []byte) error {
	var rows []apiHealthCheck
	if err := json.Unmarshal(data, &rows); err == nil {
		*c = rows
		return nil
	}

	// Dashboard has returned an array since at least Reef. Retain the keyed
	// form as a parser fallback for older proxies or saved API documents.
	var keyed map[string]apiHealthCheck
	if err := json.Unmarshal(data, &keyed); err != nil {
		return fmt.Errorf("decode health checks: %w", err)
	}
	rows = make([]apiHealthCheck, 0, len(keyed))
	for code, check := range keyed {
		if check.Type == "" {
			check.Type = code
		}
		rows = append(rows, check)
	}
	*c = rows
	return nil
}

func (d funcDepsAdapter) Health(ctx context.Context, limit int) (cephfunc.HealthResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.HealthResult{}, err
	}
	var response struct {
		Health *struct {
			Checks *apiHealthChecks `json:"checks"`
		} `json:"health"`
	}
	if err := d.collector.apiClient.getJSON(ctx, "get health detail", urlPathApiHealthMinimal, hdrAcceptVersion, nil, &response); err != nil {
		return cephfunc.HealthResult{}, toFunctionSourceError(err, "health detail")
	}

	if response.Health == nil || response.Health.Checks == nil {
		return cephfunc.HealthResult{}, errors.New("health detail response is missing health checks")
	}
	checks := append(apiHealthChecks(nil), (*response.Health.Checks)...)
	seenCheckTypes := make(map[string]bool, len(checks))
	for _, check := range checks {
		if check.Type == "" {
			return cephfunc.HealthResult{}, errors.New("health detail returned a check without a type")
		}
		if seenCheckTypes[check.Type] {
			return cephfunc.HealthResult{}, errors.New("health detail returned duplicate check types")
		}
		if check.Summary.Count < 0 {
			return cephfunc.HealthResult{}, errors.New("health detail returned a negative affected-item count")
		}
		seenCheckTypes[check.Type] = true
	}
	sort.SliceStable(checks, func(i, j int) bool {
		left := cephHealthSeverityRank(checks[i].Severity)
		right := cephHealthSeverityRank(checks[j].Severity)
		if left != right {
			return left > right
		}
		return checks[i].Type < checks[j].Type
	})
	result := cephfunc.HealthResult{}
	target := limit + 1
	for _, check := range checks {
		code := check.Type
		severity := strings.ToUpper(check.Severity)
		if len(check.Detail) == 0 {
			result.Total++
			if len(result.Rows) < target {
				result.Rows = append(result.Rows, cephfunc.HealthRow{
					ID:       code + "#00000000",
					Code:     code,
					Severity: severity,
					Muted:    check.Muted,
					Summary:  check.Summary.Message,
					Count:    check.Summary.Count,
				})
			}
			continue
		}
		for index, detail := range check.Detail {
			result.Total++
			if len(result.Rows) < target {
				result.Rows = append(result.Rows, cephfunc.HealthRow{
					ID:       fmt.Sprintf("%s#%08d", code, index),
					Code:     code,
					Severity: severity,
					Muted:    check.Muted,
					Summary:  check.Summary.Message,
					Count:    check.Summary.Count,
					Detail:   detail.Message,
				})
			}
		}
	}
	return result, nil
}

func (d funcDepsAdapter) OSDs(ctx context.Context, limit int) (cephfunc.OSDResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.OSDResult{}, err
	}
	if limit <= 0 || limit > cephfunc.MaxInventoryLimit {
		return cephfunc.OSDResult{}, errors.New("OSD inventory received an invalid row limit")
	}
	query := url.Values{
		"offset": {"0"},
		"limit":  {strconv.Itoa(limit)},
		"sort":   {"+id"},
	}
	var osds []apiOsdResponse
	headers, err := d.collector.apiClient.getJSONWithHeaders(
		ctx,
		"list OSDs for Function",
		urlPathApiOsd,
		hdrAcceptVersionV11,
		query,
		&osds,
	)
	if err != nil {
		return cephfunc.OSDResult{}, toFunctionSourceError(err, "OSD inventory")
	}
	total, err := strconv.Atoi(headers.Get("X-Total-Count"))
	if err != nil || total < 0 {
		return cephfunc.OSDResult{}, errors.New("OSD inventory returned an invalid X-Total-Count")
	}
	if total > cephfunc.MaxInventoryLimit {
		return cephfunc.OSDResult{}, &cephfunc.InventoryLimitError{
			Resource: "OSD",
			Total:    total,
			Limit:    cephfunc.MaxInventoryLimit,
			Hard:     true,
		}
	}
	if total > limit {
		return cephfunc.OSDResult{}, &cephfunc.InventoryLimitError{
			Resource: "OSD",
			Total:    total,
			Limit:    limit,
		}
	}
	if len(osds) != total {
		return cephfunc.OSDResult{}, &cephfunc.IncompleteInventoryError{
			Resource: "OSD",
			Rows:     len(osds),
			Total:    total,
		}
	}
	if err := validateOSDs(osds); err != nil {
		return cephfunc.OSDResult{}, err
	}

	rows := make([]cephfunc.OSDRow, 0, len(osds))
	for _, osd := range osds {
		totalBytes := osd.OsdStats.Statfs.Total
		availableBytes := osd.OsdStats.Statfs.Available
		usedBytes := totalBytes - availableBytes
		utilization := float64(0)
		if totalBytes > 0 {
			utilization = float64(usedBytes) / float64(totalBytes) * 100
		}
		name := osd.Tree.Name
		if name == "" {
			name = fmt.Sprintf("osd.%d", osd.ID)
		}
		rows = append(rows, cephfunc.OSDRow{
			ID:                osd.ID,
			Name:              name,
			UUID:              osd.UUID,
			Host:              osd.Host.Name,
			DeviceClass:       osd.Tree.DeviceClass,
			Up:                osd.Up == 1,
			In:                osd.In == 1,
			OperationalStatus: osd.OperationalStatus,
			TotalBytes:        totalBytes,
			UsedBytes:         usedBytes,
			AvailableBytes:    availableBytes,
			Utilization:       utilization,
			ReadBytesPerSec:   osd.Stats.OpOutBytes,
			WriteBytesPerSec:  osd.Stats.OpInBytes,
			ReadOpsPerSec:     osd.Stats.OpR,
			WriteOpsPerSec:    osd.Stats.OpW,
			CommitLatencyMS:   osd.OsdStats.PerfStat.CommitLatencyMs,
			ApplyLatencyMS:    osd.OsdStats.PerfStat.ApplyLatencyMs,
		})
	}
	return cephfunc.OSDResult{
		Rows:  rows,
		Total: total,
	}, nil
}

func (d funcDepsAdapter) Pools(ctx context.Context, limit int) (cephfunc.PoolResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.PoolResult{}, err
	}
	if limit <= 0 || limit > cephfunc.MaxInventoryLimit {
		return cephfunc.PoolResult{}, errors.New("pool inventory received an invalid row limit")
	}
	attrs := strings.Join([]string{
		"pool_name", "type", "size", "min_size", "pg_num", "pg_placement_num", "pg_autoscale_mode",
		"crush_rule", "application_metadata", "erasure_code_profile", "quota_max_bytes",
		"quota_max_objects", "flags_names",
	}, ",")
	var pools []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list pool policy", urlPathApiPool, hdrAcceptVersion,
		url.Values{
			"stats": {"false"},
			"attrs": {attrs},
		}, &pools); err != nil {
		return cephfunc.PoolResult{}, toFunctionSourceError(err, "pool inventory")
	}
	if len(pools) > cephfunc.MaxInventoryLimit {
		return cephfunc.PoolResult{}, &cephfunc.InventoryLimitError{
			Resource: "pool",
			Total:    len(pools),
			Limit:    cephfunc.MaxInventoryLimit,
			Hard:     true,
		}
	}
	if len(pools) > limit {
		return cephfunc.PoolResult{}, &cephfunc.InventoryLimitError{
			Resource: "pool",
			Total:    len(pools),
			Limit:    limit,
		}
	}
	seenPools := make(map[string]bool, len(pools))
	for _, pool := range pools {
		name := anyString(pool["pool_name"])
		if name == "" {
			return cephfunc.PoolResult{}, errors.New("pool inventory returned an empty pool name")
		}
		if seenPools[name] {
			return cephfunc.PoolResult{}, errors.New("pool inventory returned duplicate pool names")
		}
		seenPools[name] = true
	}
	var rules []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list CRUSH rules", urlPathAPICrushRule, hdrAcceptVersionV2, nil, &rules); err != nil {
		return cephfunc.PoolResult{}, toFunctionSourceError(err, "CRUSH rule inventory")
	}
	if len(rules) > cephfunc.MaxInventoryLimit {
		return cephfunc.PoolResult{}, &cephfunc.InventoryLimitError{
			Resource: "CRUSH rule",
			Total:    len(rules),
			Limit:    cephfunc.MaxInventoryLimit,
			Hard:     true,
		}
	}
	ruleInfo := make(map[string]crushRuleInfo, len(rules))
	for _, rule := range rules {
		name := anyString(rule["rule_name"])
		if name != "" {
			if _, ok := ruleInfo[name]; ok {
				return cephfunc.PoolResult{}, errors.New("CRUSH rule inventory returned duplicate rule names")
			}
			ruleInfo[name] = parseCrushRule(rule)
		}
	}

	rows := make([]cephfunc.PoolRow, 0, len(pools))
	for _, pool := range pools {
		size, err := nonnegativeInt64Field(pool, "size")
		if err != nil {
			return cephfunc.PoolResult{}, err
		}
		minSize, err := nonnegativeInt64Field(pool, "min_size")
		if err != nil || size != nil && minSize != nil && *size > 0 && *minSize > *size {
			return cephfunc.PoolResult{}, errors.New("pool inventory returned invalid replica/shard counts")
		}
		pgNum, err := nonnegativeInt64Field(pool, "pg_num")
		if err != nil {
			return cephfunc.PoolResult{}, err
		}
		pgpNum, err := nonnegativeInt64Field(pool, "pg_placement_num")
		if err != nil {
			return cephfunc.PoolResult{}, err
		}
		quotaMaxBytes, err := nonnegativeInt64Field(pool, "quota_max_bytes")
		if err != nil {
			return cephfunc.PoolResult{}, err
		}
		quotaMaxObjects, err := nonnegativeInt64Field(pool, "quota_max_objects")
		if err != nil {
			return cephfunc.PoolResult{}, err
		}
		ruleName := anyString(pool["crush_rule"])
		placement := ruleInfo[ruleName]
		rows = append(rows, cephfunc.PoolRow{
			Name:            anyString(pool["pool_name"]),
			Type:            anyString(pool["type"]),
			Size:            size,
			MinSize:         minSize,
			PGNum:           pgNum,
			PGPNum:          pgpNum,
			PGAutoscaleMode: anyString(pool["pg_autoscale_mode"]),
			CrushRule:       ruleName,
			CrushRoot:       placement.root,
			FailureDomain:   placement.failureDomain,
			DeviceClass:     placement.deviceClass,
			Applications:    strings.Join(anyStringSlice(pool["application_metadata"]), ","),
			ErasureProfile:  anyString(pool["erasure_code_profile"]),
			QuotaMaxBytes:   quotaMaxBytes,
			QuotaMaxObjects: quotaMaxObjects,
			Flags:           strings.Join(anyStringSlice(pool["flags_names"]), ","),
		})
	}
	return cephfunc.PoolResult{
		Rows:  rows,
		Total: len(pools),
	}, nil
}

func (d funcDepsAdapter) Daemons(ctx context.Context, limit int) (cephfunc.DaemonResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.DaemonResult{}, err
	}
	if limit <= 0 || limit > cephfunc.MaxInventoryLimit {
		return cephfunc.DaemonResult{}, errors.New("daemon inventory received an invalid row limit")
	}
	var daemons []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list daemons", urlPathAPIDaemon, hdrAcceptVersion, nil, &daemons); err != nil {
		return cephfunc.DaemonResult{}, toFunctionSourceError(err, "daemon inventory")
	}
	if len(daemons) > cephfunc.MaxInventoryLimit {
		return cephfunc.DaemonResult{}, &cephfunc.InventoryLimitError{
			Resource: "daemon",
			Total:    len(daemons),
			Limit:    cephfunc.MaxInventoryLimit,
			Hard:     true,
		}
	}
	if len(daemons) > limit {
		return cephfunc.DaemonResult{}, &cephfunc.InventoryLimitError{
			Resource: "daemon",
			Total:    len(daemons),
			Limit:    limit,
		}
	}
	seenDaemons := make(map[string]bool, len(daemons))
	for _, daemon := range daemons {
		id, name := daemonIdentity(daemon)
		if id == "" || id == "." || name == "" {
			return cephfunc.DaemonResult{}, errors.New("daemon inventory returned an incomplete daemon identity")
		}
		if seenDaemons[id] {
			return cephfunc.DaemonResult{}, errors.New("daemon inventory returned duplicate daemon identities")
		}
		seenDaemons[id] = true
	}
	rows := make([]cephfunc.DaemonRow, 0, len(daemons))
	for _, daemon := range daemons {
		typ := anyString(daemon["daemon_type"])
		id, name := daemonIdentity(daemon)
		var active *bool
		if rawActive, exists := daemon["is_active"]; exists && rawActive != nil {
			value, ok := anyBoolKnown(rawActive)
			if !ok {
				return cephfunc.DaemonResult{}, errors.New("daemon inventory returned an invalid active state")
			}
			active = new(value)
		}
		rows = append(rows, cephfunc.DaemonRow{
			ID:     id,
			Type:   typ,
			Name:   name,
			Host:   anyString(daemon["hostname"]),
			Status: anyString(daemon["status_desc"]),
			Active: active,
			Version: anyString(
				daemon["version"],
			),
			Image: firstNonEmpty(
				anyString(daemon["container_image_name"]),
				anyString(daemon["container_image_id"]),
			),
			LastRefresh: anyString(daemon["last_refresh"]),
			Placement:   anyString(daemon["service_name"]),
		})
	}
	return cephfunc.DaemonResult{
		Rows:  rows,
		Total: len(daemons),
	}, nil
}

type crushRuleInfo struct{ root, failureDomain, deviceClass string }

func cephHealthSeverityRank(severity string) int {
	switch strings.ToUpper(severity) {
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

func daemonIdentity(daemon map[string]any) (id, name string) {
	typ := anyString(daemon["daemon_type"])
	id = anyString(daemon["daemon_name"])
	name = anyString(daemon["daemon_id"])
	if name == "" {
		name = id
	}
	if id == "" {
		id = typ + "." + name
	}
	return id, name
}

func parseCrushRule(rule map[string]any) crushRuleInfo {
	var result crushRuleInfo
	steps, _ := rule["steps"].([]any)
	for _, rawStep := range steps {
		step := anyMap(rawStep)
		op := anyString(step["op"])
		switch {
		case op == "take":
			result.root = firstNonEmpty(anyString(step["item_name"]), anyString(step["item"]))
			if before, after, ok := strings.Cut(result.root, "~"); ok {
				result.root = before
				result.deviceClass = after
			}
		case strings.HasPrefix(op, "choose"):
			if typ := anyString(step["type"]); typ != "" {
				result.failureDomain = typ
			}
		}
	}
	return result
}

func toFunctionSourceError(err error, capability string) error {
	var apiErr *apiHTTPError
	if errors.As(err, &apiErr) {
		message := capability + " is unavailable"
		switch apiErr.status {
		case http.StatusForbidden:
			message = "Dashboard permission is insufficient for " + capability
		case http.StatusNotFound:
			message = capability + " is unsupported by this Dashboard release or configuration"
		case http.StatusServiceUnavailable:
			message = capability + " is unavailable; check its Ceph service prerequisite"
		}
		return &cephfunc.SourceError{
			Status:  apiErr.status,
			Message: message,
		}
	}
	return err
}

func anyMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func anyString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func anyBoolKnown(value any) (bool, bool) {
	switch value := value.(type) {
	case bool:
		return value, true
	case json.Number:
		result, err := strconv.ParseFloat(value.String(), 64)
		return result == 1, err == nil && (result == 0 || result == 1)
	case string:
		result, err := strconv.ParseBool(strings.TrimSpace(value))
		return result, err == nil
	default:
		return false, false
	}
}

func anyStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if stringValue := anyString(value); stringValue != "" {
				result = append(result, stringValue)
			}
		}
		return result
	case string:
		if values != "" {
			return []string{values}
		}
	}
	return nil
}

func exactInt64Field(values map[string]any, keys ...string) (*int64, error) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		result, err := exactInt64(value)
		if err != nil {
			return nil, err
		}
		return new(result), nil
	}
	return nil, nil
}

func nonnegativeInt64Field(values map[string]any, key string) (*int64, error) {
	value, err := exactInt64Field(values, key)
	if err != nil || value != nil && *value < 0 {
		return nil, fmt.Errorf("pool inventory returned an invalid %s value", key)
	}
	return value, nil
}

func exactInt64(value any) (int64, error) {
	switch value := value.(type) {
	case int:
		return int64(value), nil
	case int64:
		return value, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < float64(math.MinInt64) || value > maxInt64Float || math.Trunc(value) != value {
			return 0, errors.New("value is not an exact 64-bit integer")
		}
		return int64(value), nil
	case json.Number:
		result, err := value.Int64()
		if err != nil {
			return 0, errors.New("value is not an exact 64-bit integer")
		}
		return result, nil
	case string:
		result, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, errors.New("value is not an exact 64-bit integer")
		}
		return result, nil
	default:
		return 0, errors.New("value is not an exact 64-bit integer")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
