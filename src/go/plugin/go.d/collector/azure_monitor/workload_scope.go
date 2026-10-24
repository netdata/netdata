// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

const (
	azureWorkloadScopeLabelKey   = "_vnode_type"
	azureWorkloadScopeLabelValue = "azure_workload"
	azureWorkloadGUIDPrefix      = "azure_monitor:"
)

type workloadScopeReport struct {
	unsafeValues map[string]int
}

func applyWorkloadHostScopes(resources []resourceInfo, runtime *collectorRuntime) workloadScopeReport {
	if runtime == nil || runtime.WorkloadResourceTagKey == "" {
		return workloadScopeReport{}
	}

	var report workloadScopeReport
	for i := range resources {
		scope, unsafeValue := workloadHostScope(resources[i], runtime.WorkloadResourceTagKey)
		resources[i].HostScope = scope
		if unsafeValue == "" {
			continue
		}
		if report.unsafeValues == nil {
			report.unsafeValues = make(map[string]int)
		}
		report.unsafeValues[unsafeValue]++
	}
	return report
}

func workloadHostScope(resource resourceInfo, tagKey string) (metrix.HostScope, string) {
	value := workloadTagValue(resource.Tags, tagKey)
	if value == "" {
		return metrix.HostScope{}, ""
	}

	guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(azureWorkloadGUIDPrefix+value)).String()
	scope := metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: value,
		Labels: map[string]string{
			azureWorkloadScopeLabelKey: azureWorkloadScopeLabelValue,
		},
	}
	if _, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     scope.GUID,
		Hostname: scope.Hostname,
		Labels:   scope.Labels,
	}); err != nil {
		return metrix.HostScope{}, value
	}
	return scope, ""
}

func workloadTagValue(tags []resourceTag, tagKey string) string {
	tagKey = stringsLowerTrim(tagKey)
	if tagKey == "" {
		return ""
	}
	for _, tag := range tags {
		if tag.Key != tagKey {
			continue
		}
		return stringsTrim(tag.Value)
	}
	return ""
}

func (c *Collector) warnDiscoveryScopeFallbacks(state discoveryState, runtime *collectorRuntime) {
	if runtime == nil || runtime.WorkloadResourceTagKey == "" || state.FetchCounter == 0 {
		return
	}

	if state.QueryTagsColumnMissing && (!c.tagsColumnMissingWarned || c.tagsColumnMissingWarnedAt != state.FetchCounter) {
		c.tagsColumnMissingWarned = true
		c.tagsColumnMissingWarnedAt = state.FetchCounter
		c.Warningf(
			"virtual_nodes.by_resource_tag %q is enabled, but the custom discovery query did not return a tags column; resources from that query will use the default host scope",
			runtime.WorkloadResourceTagKey,
		)
	}
	if state.QueryTagsWrongShape && (!c.tagsWrongShapeWarned || c.tagsWrongShapeWarnedAt != state.FetchCounter) {
		c.tagsWrongShapeWarned = true
		c.tagsWrongShapeWarnedAt = state.FetchCounter
		c.Warningf(
			"virtual_nodes.by_resource_tag %q is enabled, but the custom discovery query returned non-object tags; affected resources will use the default host scope",
			runtime.WorkloadResourceTagKey,
		)
	}
	if len(state.UnsafeWorkloadValues) == 0 {
		return
	}
	c.Warningf(
		"virtual_nodes.by_resource_tag %q ignored unsafe tag values; affected resources will use the default host scope: %s",
		runtime.WorkloadResourceTagKey,
		formatUnsafeWorkloadValues(state.UnsafeWorkloadValues),
	)
}

func formatUnsafeWorkloadValues(values map[string]int) string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, value := range keys {
		parts = append(parts, fmt.Sprintf("%s (%d resources)", strconv.Quote(value), values[value]))
	}
	return strings.Join(parts, ", ")
}
