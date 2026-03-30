// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func (c *Collector) refreshDiscovery(ctx context.Context, force bool) ([]resourceInfo, error) {
	now := c.now()
	if !force && !c.discovery.FetchedAt.IsZero() && now.Before(c.discovery.ExpiresAt) {
		return c.discovery.Resources, nil
	}

	resources, byType, err := c.discoverResources(ctx)
	if err != nil {
		return nil, err
	}

	if !slices.Equal(resources, c.discovery.Resources) {
		c.Infof("discovered %d resources: %v", len(resources), resources)
	}

	c.discovery = discoveryState{
		Resources:    resources,
		ByType:       byType,
		FetchedAt:    now,
		ExpiresAt:    now.Add(secondsToDuration(c.DiscoveryEvery)),
		FetchCounter: c.discovery.FetchCounter + 1,
	}

	return resources, nil
}

// discoverResourceTypes queries Azure Resource Graph to find all resource types
// present in the subscription. Used by auto-discovery to determine which profiles to activate.
func discoverResourceTypes(ctx context.Context, subscriptionID string, timeout time.Duration, resourceGraph resourceGraphClient) (map[string]struct{}, error) {
	query := "resources | summarize count() by type"
	types := make(map[string]struct{})

	var skipToken *string
	for {
		req := armResourceGraphQuery(subscriptionID, query, skipToken)
		reqCtx, cancel := withOptionalTimeout(ctx, timeout)
		resp, err := resourceGraph.Resources(reqCtx, req, nil)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("resource graph query: %w", err)
		}

		rows, err := parseResourceGraphObjectArray(resp.Data)
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			t := stringsLowerTrim(asString(row["type"]))
			if t != "" {
				types[t] = struct{}{}
			}
		}

		if resp.SkipToken == nil || stringsTrim(*resp.SkipToken) == "" {
			break
		}
		token := stringsTrim(*resp.SkipToken)
		skipToken = &token
	}

	return types, nil
}

func (c *Collector) discoverResources(ctx context.Context) ([]resourceInfo, map[string][]resourceInfo, error) {
	resourceTypes := make([]string, 0, len(c.runtime.Profiles))
	seenTypes := map[string]struct{}{}
	for _, p := range c.runtime.Profiles {
		t := stringsTrim(p.ResourceType)
		if t == "" {
			continue
		}
		tLower := stringsLowerTrim(t)
		if _, ok := seenTypes[tLower]; ok {
			continue
		}
		seenTypes[tLower] = struct{}{}
		resourceTypes = append(resourceTypes, t)
	}

	if len(resourceTypes) == 0 {
		return nil, map[string][]resourceInfo{}, nil
	}

	query := buildDiscoveryQuery(resourceTypes)
	if query == "" {
		return nil, nil, fmt.Errorf("failed to build resource discovery query")
	}

	resourceGroupsFilter := make(map[string]struct{}, len(c.ResourceGroups))
	for _, rg := range c.ResourceGroups {
		n := stringsLowerTrim(rg)
		if n == "" {
			continue
		}
		resourceGroupsFilter[n] = struct{}{}
	}

	result := make([]resourceInfo, 0, 256)
	seenIDs := make(map[string]struct{})

	var skipToken *string
	for {
		req := armResourceGraphQuery(c.SubscriptionID, query, skipToken)
		reqCtx, cancel := withOptionalTimeout(ctx, c.Timeout.Duration())
		resp, err := c.resourceGraph.Resources(reqCtx, req, nil)
		cancel()
		if err != nil {
			return nil, nil, err
		}

		rows, err := parseResourceGraphObjectArray(resp.Data)
		if err != nil {
			return nil, nil, err
		}

		for _, row := range rows {
			id := stringsTrim(asString(row["id"]))
			if id == "" {
				continue
			}
			if _, ok := seenIDs[id]; ok {
				continue
			}
			seenIDs[id] = struct{}{}

			rg := stringsTrim(asString(row["resourceGroup"]))
			rgLower := stringsLowerTrim(rg)
			if len(resourceGroupsFilter) > 0 {
				if _, ok := resourceGroupsFilter[rgLower]; !ok {
					continue
				}
			}

			resourceType := stringsTrim(asString(row["type"]))
			if resourceType == "" {
				continue
			}
			region := stringsLowerTrim(asString(row["location"]))
			if region == "" {
				region = "global"
			}

			result = append(result, resourceInfo{
				ID:            id,
				UID:           hashShort(id),
				Name:          stringsTrim(asString(row["name"])),
				Type:          resourceType,
				ResourceGroup: rg,
				Region:        region,
			})
		}

		if resp.SkipToken == nil || stringsTrim(*resp.SkipToken) == "" {
			break
		}
		token := stringsTrim(*resp.SkipToken)
		skipToken = &token
	}

	byType := make(map[string][]resourceInfo)
	for _, r := range result {
		key := stringsLowerTrim(r.Type)
		byType[key] = append(byType[key], r)
	}

	return result, byType, nil
}

func armResourceGraphQuery(subscriptionID, query string, skipToken *string) armresourcegraph.QueryRequest {
	resultFormat := armresourcegraph.ResultFormatObjectArray
	top := int32(1000)
	options := &armresourcegraph.QueryRequestOptions{
		ResultFormat: &resultFormat,
		Top:          &top,
	}
	if skipToken != nil && stringsTrim(*skipToken) != "" {
		token := stringsTrim(*skipToken)
		options.SkipToken = &token
	}

	subID := stringsTrim(subscriptionID)
	return armresourcegraph.QueryRequest{
		Query:         &query,
		Subscriptions: []*string{&subID},
		Options:       options,
	}
}

func buildDiscoveryQuery(resourceTypes []string) string {
	if len(resourceTypes) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(resourceTypes))
	for _, rt := range resourceTypes {
		rt = stringsTrim(rt)
		if rt == "" {
			continue
		}
		if !azureprofiles.IsValidResourceType(rt) {
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(rt, "'", "''")+"'")
	}
	if len(quoted) == 0 {
		return ""
	}
	return "resources | where type in~ (" + strings.Join(quoted, ",") + ") | project id, name, type, resourceGroup, location"
}

func parseResourceGraphObjectArray(v any) ([]map[string]any, error) {
	rows, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected resource graph result format: %T", v)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return ""
	}
}
