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
	urlPathAPICrushRule      = "/api/crush_rule"
	urlPathAPIDaemon         = "/api/daemon"
	urlPathAPIRGWDaemon      = "/api/rgw/daemon"
	urlPathAPIRGWRealm       = "/api/rgw/realm"
	urlPathAPIRGWZonegroup   = "/api/rgw/zonegroup"
	urlPathAPIRGWZone        = "/api/rgw/zone"
	urlPathAPIRGWSyncStatus  = "/api/rgw/multisite/sync_status"
	urlPathAPIRGWUser        = "/api/rgw/user"
	urlPathAPIRGWBucket      = "/api/rgw/bucket"
	urlPathAPIRGWAccounts    = "/api/rgw/accounts"
	hdrAcceptVersionV2       = "application/vnd.ceph.api.v2.0+json"
	maxFunctionRows          = 10000
	maxFunctionLookaheadRows = maxFunctionRows + 1
	maxRGWSyncProbes         = 10
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
	target := functionRowTarget(limit)
	for _, check := range checks {
		code := check.Type
		severity := strings.ToUpper(check.Severity)
		if len(check.Detail) == 0 {
			result.Total++
			if len(result.Rows) < target {
				result.Rows = append(result.Rows, cephfunc.HealthRow{
					ID: code + "#00000000", Code: code, Severity: severity, Muted: check.Muted,
					Summary: check.Summary.Message, Count: check.Summary.Count,
				})
			}
			continue
		}
		for index, detail := range check.Detail {
			result.Total++
			if len(result.Rows) < target {
				result.Rows = append(result.Rows, cephfunc.HealthRow{
					ID: fmt.Sprintf("%s#%08d", code, index), Code: code, Severity: severity,
					Muted: check.Muted, Summary: check.Summary.Message, Count: check.Summary.Count,
					Detail: detail.Message,
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
	target := functionRowTarget(limit)
	var osds []apiOsdResponse
	total := 0
	for offset := 0; len(osds) < target; {
		pageLimit := min(osdPageSize, target-len(osds))
		query := url.Values{
			"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(pageLimit)}, "sort": {"+id"},
		}
		var page []apiOsdResponse
		headers, err := d.collector.apiClient.getJSONWithHeaders(ctx, "list OSDs for Function", urlPathApiOsd, hdrAcceptVersionV11, query, &page)
		if err != nil {
			return cephfunc.OSDResult{}, toFunctionSourceError(err, "OSD inventory")
		}
		if raw := headers.Get("X-Total-Count"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 {
				return cephfunc.OSDResult{}, errors.New("OSD inventory returned an invalid X-Total-Count")
			}
			total = parsed
		}
		pageLength := len(page)
		if total < offset+pageLength {
			total = offset + pageLength
		}
		if remaining := target - len(osds); len(page) > remaining {
			page = page[:remaining]
		}
		osds = append(osds, page...)
		offset += pageLength
		if pageLength < pageLimit {
			if total > offset {
				return cephfunc.OSDResult{}, errors.New("OSD inventory returned a short page before the advertised total")
			}
			break
		}
	}
	if total < len(osds) {
		total = len(osds)
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
			ID: osd.ID, Name: name, UUID: osd.UUID, Host: osd.Host.Name,
			DeviceClass: osd.Tree.DeviceClass, Up: osd.Up == 1, In: osd.In == 1,
			OperationalStatus: osd.OperationalStatus, TotalBytes: totalBytes, UsedBytes: usedBytes,
			AvailableBytes: availableBytes, Utilization: utilization,
			ReadBytesPerSec: osd.Stats.OpOutBytes, WriteBytesPerSec: osd.Stats.OpInBytes,
			ReadOpsPerSec: osd.Stats.OpR, WriteOpsPerSec: osd.Stats.OpW,
			CommitLatencyMS: osd.OsdStats.PerfStat.CommitLatencyMs,
			ApplyLatencyMS:  osd.OsdStats.PerfStat.ApplyLatencyMs,
		})
	}
	return cephfunc.OSDResult{Rows: rows, Total: total}, nil
}

func (d funcDepsAdapter) Pools(ctx context.Context, limit int) (cephfunc.PoolResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.PoolResult{}, err
	}
	attrs := strings.Join([]string{
		"pool_name", "type", "size", "min_size", "pg_num", "pg_placement_num", "pg_autoscale_mode",
		"crush_rule", "application_metadata", "erasure_code_profile", "quota_max_bytes",
		"quota_max_objects", "flags_names",
	}, ",")
	var pools []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list pool policy", urlPathApiPool, hdrAcceptVersion,
		url.Values{"stats": {"false"}, "attrs": {attrs}}, &pools); err != nil {
		return cephfunc.PoolResult{}, toFunctionSourceError(err, "pool inventory")
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
	sort.SliceStable(pools, func(i, j int) bool {
		return anyString(pools[i]["pool_name"]) < anyString(pools[j]["pool_name"])
	})
	var rules []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list CRUSH rules", urlPathAPICrushRule, hdrAcceptVersionV2, nil, &rules); err != nil {
		return cephfunc.PoolResult{}, toFunctionSourceError(err, "CRUSH rule inventory")
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

	target := functionRowTarget(limit)
	rows := make([]cephfunc.PoolRow, 0, min(len(pools), target))
	for _, pool := range pools {
		if len(rows) == target {
			break
		}
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
			Name: anyString(pool["pool_name"]), Type: anyString(pool["type"]),
			Size: size, MinSize: minSize,
			PGNum: pgNum, PGPNum: pgpNum,
			PGAutoscaleMode: anyString(pool["pg_autoscale_mode"]), CrushRule: ruleName,
			CrushRoot: placement.root, FailureDomain: placement.failureDomain, DeviceClass: placement.deviceClass,
			Applications:   strings.Join(anyStringSlice(pool["application_metadata"]), ","),
			ErasureProfile: anyString(pool["erasure_code_profile"]),
			QuotaMaxBytes:  quotaMaxBytes, QuotaMaxObjects: quotaMaxObjects,
			Flags: strings.Join(anyStringSlice(pool["flags_names"]), ","),
		})
	}
	return cephfunc.PoolResult{Rows: rows, Total: len(pools)}, nil
}

func (d funcDepsAdapter) Daemons(ctx context.Context, limit int) (cephfunc.DaemonResult, error) {
	if err := d.revalidateClusterIdentity(ctx); err != nil {
		return cephfunc.DaemonResult{}, err
	}
	var daemons []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list daemons", urlPathAPIDaemon, hdrAcceptVersion, nil, &daemons); err != nil {
		return cephfunc.DaemonResult{}, toFunctionSourceError(err, "daemon inventory")
	}
	sort.SliceStable(daemons, func(i, j int) bool {
		leftID, leftName := daemonIdentity(daemons[i])
		rightID, rightName := daemonIdentity(daemons[j])
		if leftID != rightID {
			return leftID < rightID
		}
		return leftName < rightName
	})
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
	target := functionRowTarget(limit)
	rows := make([]cephfunc.DaemonRow, 0, min(len(daemons), target))
	for _, daemon := range daemons {
		if len(rows) == target {
			break
		}
		typ := anyString(daemon["daemon_type"])
		id, name := daemonIdentity(daemon)
		var active *bool
		if rawActive, exists := daemon["is_active"]; exists && rawActive != nil {
			value, ok := anyBoolKnown(rawActive)
			if !ok {
				return cephfunc.DaemonResult{}, errors.New("daemon inventory returned an invalid active state")
			}
			active = boolPtr(value)
		}
		rows = append(rows, cephfunc.DaemonRow{
			ID: id, Type: typ, Name: name, Host: anyString(daemon["hostname"]),
			Status: anyString(daemon["status_desc"]), Active: active,
			Version: anyString(daemon["version"]), Image: firstNonEmpty(anyString(daemon["container_image_name"]), anyString(daemon["container_image_id"])),
			LastRefresh: anyString(daemon["last_refresh"]), Placement: anyString(daemon["service_name"]),
		})
	}
	return cephfunc.DaemonResult{Rows: rows, Total: len(daemons)}, nil
}

func (d funcDepsAdapter) RGWMultisite(ctx context.Context, limit int) (cephfunc.RGWMultisiteResult, error) {
	type objectList struct {
		kind       string
		path       string
		key        string
		defaultKey string
	}
	lists := []objectList{
		{kind: "realm", path: urlPathAPIRGWRealm, key: "realms", defaultKey: "default_info"},
		{kind: "zonegroup", path: urlPathAPIRGWZonegroup, key: "zonegroups", defaultKey: "default_info"},
		{kind: "zone", path: urlPathAPIRGWZone, key: "zones", defaultKey: "default_info"},
	}
	type candidate struct {
		kind, path, name, defaultInfo string
		hasDefault                    bool
	}
	var candidates []candidate
	seenCandidates := make(map[string]bool)
	for _, list := range lists {
		var response map[string]any
		if err := d.collector.apiClient.getJSON(ctx, "list RGW "+list.kind, list.path, hdrAcceptVersion, nil, &response); err != nil {
			return cephfunc.RGWMultisiteResult{}, toFunctionSourceError(err, "RGW multisite inventory")
		}
		defaultInfo, hasDefault, err := rgwDefaultInfo(response, list.defaultKey)
		if err != nil {
			return cephfunc.RGWMultisiteResult{}, err
		}
		for _, name := range anyStringSlice(response[list.key]) {
			candidateKey := list.kind + "\x00" + name
			if seenCandidates[candidateKey] {
				return cephfunc.RGWMultisiteResult{}, errors.New("RGW multisite inventory returned duplicate object names")
			}
			seenCandidates[candidateKey] = true
			candidates = append(candidates, candidate{
				kind: list.kind, path: list.path, name: name, defaultInfo: defaultInfo,
				hasDefault: hasDefault,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank, rightRank := rgwObjectKindRank(candidates[i].kind), rgwObjectKindRank(candidates[j].kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return candidates[i].name < candidates[j].name
	})

	result := cephfunc.RGWMultisiteResult{Total: len(candidates)}
	masterZones := make(map[string]string)
	seenRowIDs := make(map[string]bool)
	for _, object := range candidates {
		if len(result.Rows) >= limit || len(result.Rows) >= maxFunctionRows {
			break
		}
		var detail map[string]any
		endpoint := endpointWithSegment(object.path, object.name)
		if err := d.collector.apiClient.getJSON(ctx, "get RGW "+object.kind, endpoint, hdrAcceptVersion, nil, &detail); err != nil {
			return cephfunc.RGWMultisiteResult{}, toFunctionSourceError(err, "RGW multisite detail")
		}
		id := firstNonEmpty(anyString(detail["id"]), object.name)
		name := firstNonEmpty(anyString(detail["name"]), object.name)
		row := cephfunc.RGWMultisiteRow{
			ID: object.kind + ":" + id, Kind: object.kind, Name: name,
			Realm:     firstNonEmpty(anyString(detail["realm_name"]), anyString(detail["realm_id"])),
			Zonegroup: firstNonEmpty(anyString(detail["zonegroup_name"]), anyString(detail["zonegroup_id"])),
			Endpoints: strings.Join(anyStringSlice(detail["endpoints"]), ","),
		}
		if seenRowIDs[row.ID] {
			return cephfunc.RGWMultisiteResult{}, errors.New("RGW multisite detail returned duplicate object identities")
		}
		seenRowIDs[row.ID] = true
		if object.hasDefault {
			row.Default = boolPtr(object.defaultInfo == id || object.defaultInfo == name)
		}
		switch object.kind {
		case "zonegroup":
			if master, ok := anyBoolKnown(detail["is_master"]); ok {
				row.Master = boolPtr(master)
			}
			if rawMasterZone, exists := detail["master_zone"]; exists && rawMasterZone != nil {
				masterZone, ok := rawMasterZone.(string)
				if !ok {
					return cephfunc.RGWMultisiteResult{}, errors.New("RGW zonegroup detail returned an invalid master-zone identity")
				}
				masterZones[id] = masterZone
				masterZones[name] = masterZone
			}
		case "zone":
			if masterZone, ok := masterZones[row.Zonegroup]; ok {
				row.Master = boolPtr(masterZone == id || masterZone == name)
			}
		}
		result.Rows = append(result.Rows, row)
	}

	var daemons []map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "list RGW daemons", urlPathAPIRGWDaemon, hdrAcceptVersion, nil, &daemons); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return cephfunc.RGWMultisiteResult{}, err
		}
		result.Total++
		if len(result.Rows) < limit {
			status, note := rgwSyncInventoryError(err)
			result.Rows = append(result.Rows, cephfunc.RGWMultisiteRow{
				ID: "sync:inventory", Kind: "sync", Name: "sync diagnostics",
				SyncStatus: status, ReleaseScope: note,
			})
		}
		return result, nil
	}

	sort.SliceStable(daemons, func(i, j int) bool {
		return rgwDaemonIdentity(daemons[i]) < rgwDaemonIdentity(daemons[j])
	})
	seenRGWDaemons := make(map[string]bool, len(daemons))
	for _, daemon := range daemons {
		name := rgwDaemonIdentity(daemon)
		if name == "" {
			return cephfunc.RGWMultisiteResult{}, errors.New("RGW daemon inventory returned an empty daemon identity")
		}
		if seenRGWDaemons[name] {
			return cephfunc.RGWMultisiteResult{}, errors.New("RGW daemon inventory returned duplicate daemon identities")
		}
		seenRGWDaemons[name] = true
	}
	probeCount := min(maxRGWSyncProbes, len(daemons))
	result.Total += probeCount
	unsupported := false
	for i := 0; i < probeCount && len(result.Rows) < limit; i++ {
		name := rgwDaemonIdentity(daemons[i])
		row := cephfunc.RGWMultisiteRow{ID: "sync:" + name, Kind: "sync", Name: name}
		if unsupported {
			row.SyncStatus = "unsupported_public_api"
			row.ReleaseScope = "Public sync API is available on Squid and Tentacle, not Reef."
			result.Rows = append(result.Rows, row)
			continue
		}
		var syncResponse map[string]any
		err := d.collector.apiClient.getJSON(ctx, "get RGW sync status", urlPathAPIRGWSyncStatus, hdrAcceptVersion,
			url.Values{"daemon_name": {name}}, &syncResponse)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return cephfunc.RGWMultisiteResult{}, err
			}
			var apiErr *apiHTTPError
			if errors.As(err, &apiErr) && apiErr.status == http.StatusNotFound {
				unsupported = true
				row.SyncStatus = "unsupported_public_api"
				row.ReleaseScope = "Public sync API is available on Squid and Tentacle, not Reef."
				result.Rows = append(result.Rows, row)
				continue
			}
			row.SyncStatus = "error"
			row.ReleaseScope = "The bounded Dashboard sync probe failed; no raw error detail is exposed."
			result.Rows = append(result.Rows, row)
			continue
		}
		row.SyncStatus = "available"
		if metadata, ok := syncResponse["metadataSyncInfo"].(map[string]any); ok {
			row.SyncStatus = firstNonEmpty(anyString(metadata["syncstatus"]), row.SyncStatus)
		}
		if bs, err := json.Marshal(syncResponse); err == nil {
			row.SyncDetail = string(bs)
		}
		row.ReleaseScope = "Best-effort Dashboard parsing of radosgw-admin output."
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

func (d funcDepsAdapter) RGWQuotas(ctx context.Context, limit int) (cephfunc.RGWQuotaResult, error) {
	targets := quotaTargets(d.collector.Functions.RGWQuotas)
	result := cephfunc.RGWQuotaResult{Total: len(targets)}
	for _, target := range targets {
		if len(result.Rows) >= limit {
			break
		}
		row, err := d.lookupRGWQuota(ctx, target.kind, target.id)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return cephfunc.RGWQuotaResult{}, err
			}
			var apiErr *apiHTTPError
			switch {
			case errors.As(err, &apiErr) && apiErr.status == http.StatusNotFound:
				status := "not_found"
				if target.kind == "account" {
					status = "unsupported_or_not_found"
				}
				result.Rows = append(result.Rows, cephfunc.RGWQuotaRow{
					Key: quotaRowKey(target.kind, target.id), ID: target.id, Kind: target.kind, Status: status,
				})
				continue
			case errors.As(err, &apiErr) && apiErr.status == http.StatusForbidden:
				return cephfunc.RGWQuotaResult{}, &cephfunc.SourceError{Status: http.StatusForbidden, Message: "Dashboard RGW read permission is required"}
			default:
				result.Rows = append(result.Rows, cephfunc.RGWQuotaRow{
					Key: quotaRowKey(target.kind, target.id), ID: target.id, Kind: target.kind, Status: "error",
				})
				continue
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

type quotaTarget struct{ kind, id string }

func quotaTargets(cfg RGWQuotasFunctionConfig) []quotaTarget {
	seen := make(map[string]bool)
	var targets []quotaTarget
	add := func(kind string, values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			key := kind + "\x00" + value
			if value == "" || seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, quotaTarget{kind: kind, id: value})
		}
	}
	add("user", cfg.Users)
	add("bucket", cfg.Buckets)
	add("account", cfg.Accounts)
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].kind != targets[j].kind {
			return targets[i].kind < targets[j].kind
		}
		return targets[i].id < targets[j].id
	})
	return targets
}

func (d funcDepsAdapter) lookupRGWQuota(ctx context.Context, kind, id string) (cephfunc.RGWQuotaRow, error) {
	path := map[string]string{"user": urlPathAPIRGWUser, "bucket": urlPathAPIRGWBucket, "account": urlPathAPIRGWAccounts}[kind]
	query := url.Values(nil)
	if kind == "user" {
		query = url.Values{"stats": {"true"}}
	}
	var response map[string]any
	if err := d.collector.apiClient.getJSON(ctx, "get RGW "+kind+" quota", endpointWithSegment(path, id), hdrAcceptVersion, query, &response); err != nil {
		return cephfunc.RGWQuotaRow{}, err
	}
	row := cephfunc.RGWQuotaRow{
		Key: quotaRowKey(kind, id), ID: id, Kind: kind, Status: "ok", Tenant: anyString(response["tenant"]),
		Account: firstNonEmpty(anyString(response["account_id"]), anyString(response["account"])),
		Owner:   firstNonEmpty(anyString(response["owner"]), anyString(response["name"])),
	}

	var usage map[string]any
	aggregateUsage := false
	var quota map[string]any
	switch kind {
	case "user":
		usage = anyMap(response["stats"])
		quota = anyMap(response["user_quota"])
		row.StatsFreshness = "Ceph RGW cached/asynchronous quota statistics; no forced sync."
	case "bucket":
		usage = anyMap(response["usage"])
		aggregateUsage = true
		quota = anyMap(response["bucket_quota"])
		row.StatsFreshness = "Ceph RGW cached/asynchronous bucket statistics; no forced sync."
	case "account":
		usage = anyMap(response["stats"])
		quota = firstNonEmptyMap(anyMap(response["account_quota"]), anyMap(response["quota"]))
		row.StatsFreshness = "Account usage availability depends on Tentacle Dashboard/RGW capabilities."
	}

	if usage != nil {
		used, objects, err := extractRGWUsage(usage, aggregateUsage)
		if err != nil {
			return cephfunc.RGWQuotaRow{}, err
		}
		row.UsedBytes = used
		row.Objects = objects
	}
	if quota != nil {
		if rawEnabled, ok := quota["enabled"]; ok && rawEnabled != nil {
			enabled, valid := anyBoolKnown(rawEnabled)
			if !valid {
				return cephfunc.RGWQuotaRow{}, errors.New("RGW quota returned an invalid enabled state")
			}
			row.QuotaEnabled = boolPtr(enabled)
		}
		maxBytes, err := exactInt64Field(quota, "max_size", "max_size_bytes")
		if err != nil || maxBytes != nil && *maxBytes < -1 {
			return cephfunc.RGWQuotaRow{}, errors.New("RGW quota returned an invalid byte limit")
		}
		maxObjects, err := exactInt64Field(quota, "max_objects")
		if err != nil || maxObjects != nil && *maxObjects < -1 {
			return cephfunc.RGWQuotaRow{}, errors.New("RGW quota returned an invalid object limit")
		}
		row.QuotaMaxBytes = maxBytes
		row.QuotaMaxObjects = maxObjects
		if row.QuotaEnabled != nil && *row.QuotaEnabled && maxBytes != nil && *maxBytes > 0 && row.UsedBytes != nil {
			utilization := float64(*row.UsedBytes) / float64(*maxBytes) * 100
			row.Utilization = &utilization
		}
	}
	return row, nil
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

func rgwDaemonIdentity(daemon map[string]any) string {
	return firstNonEmpty(anyString(daemon["id"]), anyString(daemon["daemon_name"]))
}

func rgwSyncInventoryError(err error) (status, note string) {
	var apiErr *apiHTTPError
	if errors.As(err, &apiErr) {
		switch apiErr.status {
		case http.StatusForbidden:
			return "permission_denied", "Dashboard RGW read permission is required for sync diagnostics."
		case http.StatusNotFound:
			return "unsupported_or_unavailable", "RGW daemon inventory is unsupported or unavailable in this Dashboard configuration."
		case http.StatusServiceUnavailable:
			return "unavailable", "RGW daemon inventory is unavailable; check the Ceph RGW service prerequisite."
		}
	}
	return "error", "RGW daemon inventory failed; no raw error detail is exposed."
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

func endpointWithSegment(basePath, segment string) string {
	escaped := strings.ReplaceAll(url.PathEscape(segment), ".", "%2E")
	return strings.TrimSuffix(basePath, "/") + "/" + escaped
}

func functionRowTarget(limit int) int {
	if limit <= 0 {
		return 1
	}
	if limit >= maxFunctionRows {
		return maxFunctionLookaheadRows
	}
	return limit + 1
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
		return &cephfunc.SourceError{Status: apiErr.status, Message: message}
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

func rgwObjectKindRank(kind string) int {
	switch kind {
	case "realm":
		return 0
	case "zonegroup":
		return 1
	case "zone":
		return 2
	default:
		return 3
	}
}

func rgwDefaultInfo(response map[string]any, key string) (value string, known bool, err error) {
	raw, exists := response[key]
	if !exists || raw == nil {
		return "", false, nil
	}
	switch raw := raw.(type) {
	case string:
		return raw, true, nil
	case map[string]any:
		value = firstNonEmpty(anyString(raw["id"]), anyString(raw["name"]))
		if value == "" && len(raw) > 0 {
			return "", false, errors.New("RGW multisite inventory returned invalid default-object information")
		}
		return value, true, nil
	default:
		return "", false, errors.New("RGW multisite inventory returned invalid default-object information")
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

func extractRGWUsage(usage map[string]any, aggregateCategories bool) (usedBytes, objects *int64, err error) {
	if !aggregateCategories || usage["size"] != nil || usage["size_utilized"] != nil ||
		usage["size_actual"] != nil || usage["num_objects"] != nil {
		usedBytes, err = exactInt64Field(usage, "size", "size_utilized", "size_actual")
		if err != nil || usedBytes != nil && *usedBytes < 0 {
			return nil, nil, errors.New("RGW usage returned an invalid byte count")
		}
		objects, err = exactInt64Field(usage, "num_objects")
		if err != nil || objects != nil && *objects < 0 {
			return nil, nil, errors.New("RGW usage returned an invalid object count")
		}
		return usedBytes, objects, nil
	}

	var usedTotal, objectTotal int64
	var haveUsed, haveObjects bool
	for _, raw := range usage {
		category := anyMap(raw)
		if category == nil {
			return nil, nil, errors.New("RGW bucket usage returned an invalid category")
		}
		used, err := exactInt64Field(category, "size", "size_actual")
		if err != nil || used != nil && (*used < 0 || *used > math.MaxInt64-usedTotal) {
			return nil, nil, errors.New("RGW bucket usage returned an invalid byte count")
		}
		if used != nil {
			usedTotal += *used
			haveUsed = true
		}
		count, err := exactInt64Field(category, "num_objects")
		if err != nil || count != nil && (*count < 0 || *count > math.MaxInt64-objectTotal) {
			return nil, nil, errors.New("RGW bucket usage returned an invalid object count")
		}
		if count != nil {
			objectTotal += *count
			haveObjects = true
		}
	}
	if haveUsed {
		usedBytes = int64Ptr(usedTotal)
	}
	if haveObjects {
		objects = int64Ptr(objectTotal)
	}
	return usedBytes, objects, nil
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
		return int64Ptr(result), nil
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

func quotaRowKey(kind, id string) string { return kind + ":" + id }

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

func firstNonEmptyMap(values ...map[string]any) map[string]any {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func int64Ptr(value int64) *int64 { return &value }
func boolPtr(value bool) *bool    { return &value }
