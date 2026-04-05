// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type discoveryFetchResult struct {
	Resources        []resourceInfo
	ByType           map[string][]resourceInfo
	UnsupportedTypes []string
}

type normalizedTagFilter struct {
	Key    string
	Values []string
}

type profileResourceMatcher struct {
	profileName    string
	resourceType   string
	resourceGroups map[string]struct{}
	regions        map[string]struct{}
	tagFilters     []normalizedTagFilter
}

func (c *Collector) refreshDiscovery(ctx context.Context, force bool) ([]resourceInfo, error) {
	now := c.now()
	if !force && !c.discovery.FetchedAt.IsZero() {
		if c.Discovery.RefreshEvery == 0 || now.Before(c.discovery.ExpiresAt) {
			return c.discovery.Resources, nil
		}
	}

	fetched, err := c.fetchDiscovery(ctx)
	if err != nil {
		return nil, err
	}
	if len(fetched.UnsupportedTypes) > 0 {
		c.Warningf("ignoring unsupported discovered resource types: %v", fetched.UnsupportedTypes)
	}

	state := buildDiscoveryState(fetched.Resources, c.runtime, now, c.Discovery.RefreshEvery, c.discovery.FetchCounter+1)
	if !equalResourceSlices(state.Resources, c.discovery.Resources) {
		c.Infof("discovered %d resources: %v", len(state.Resources), state.Resources)
	}

	c.discovery = state
	return state.Resources, nil
}

func buildDiscoveryState(resources []resourceInfo, runtime *collectorRuntime, now time.Time, refreshEvery int, fetchCounter uint64) discoveryState {
	filteredResources, byType := filterDiscoveryResourcesByTypes(resources, runtimeResourceTypes(runtime))
	return discoveryState{
		Resources:    filteredResources,
		ByType:       byType,
		ByProfile:    filterDiscoveryResourcesByProfiles(filteredResources, runtime),
		FetchedAt:    now,
		ExpiresAt:    discoveryExpiresAt(now, refreshEvery),
		FetchCounter: fetchCounter,
	}
}

func discoveryExpiresAt(now time.Time, refreshEvery int) time.Time {
	if refreshEvery == 0 {
		return time.Time{}
	}
	return now.Add(secondsToDuration(refreshEvery))
}

func (c *Collector) fetchDiscovery(ctx context.Context) (discoveryFetchResult, error) {
	switch stringsLowerTrim(c.Discovery.Mode) {
	case discoveryModeQuery:
		return discoverResourcesFromQuery(
			ctx,
			c.subscriptionIDs(),
			c.Timeout.Duration(),
			c.resourceGraph,
			c.Discovery.ModeQuery.KQL,
			c.supportedResourceTypes,
		)
	default:
		resources, byType, err := discoverResources(
			ctx,
			c.subscriptionIDs(),
			c.Timeout.Duration(),
			c.resourceGraph,
			runtimeResourceTypes(c.runtime),
			c.Discovery.ModeFilters,
		)
		if err != nil {
			return discoveryFetchResult{}, err
		}
		return discoveryFetchResult{Resources: resources, ByType: byType}, nil
	}
}

func runtimeResourceTypes(runtime *collectorRuntime) []string {
	if runtime == nil || len(runtime.Profiles) == 0 {
		return nil
	}

	resourceTypes := make([]string, 0, len(runtime.Profiles))
	seenTypes := make(map[string]struct{}, len(runtime.Profiles))
	for _, p := range runtime.Profiles {
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
	return resourceTypes
}

func discoverResources(ctx context.Context, subscriptionIDs []string, timeout time.Duration, resourceGraph resourceGraphClient, resourceTypes []string, filters *ResourceFiltersConfig) ([]resourceInfo, map[string][]resourceInfo, error) {
	if len(resourceTypes) == 0 {
		return nil, map[string][]resourceInfo{}, nil
	}

	query := buildDiscoveryQuery(resourceTypes, filters)
	if query == "" {
		return nil, nil, fmt.Errorf("failed to build resource discovery query")
	}

	resourceGroupsFilter := normalizedFilterSet(nil)
	regionsFilter := normalizedFilterSet(nil)
	if filters != nil {
		resourceGroupsFilter = normalizedFilterSet(filters.ResourceGroups)
		regionsFilter = normalizedFilterSet(filters.Regions)
	}

	result := make([]resourceInfo, 0, 256)
	seenIDs := make(map[string]struct{})

	var skipToken *string
	for {
		req := armResourceGraphQuery(subscriptionIDs, query, skipToken)
		reqCtx, cancel := withOptionalTimeout(ctx, timeout)
		resp, err := resourceGraph.Resources(reqCtx, req, nil)
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
			idKey := stringsLowerTrim(id)
			if _, ok := seenIDs[idKey]; ok {
				continue
			}
			seenIDs[idKey] = struct{}{}

			rg := stringsTrim(asString(row["resourceGroup"]))
			if len(resourceGroupsFilter) > 0 {
				if _, ok := resourceGroupsFilter[stringsLowerTrim(rg)]; !ok {
					continue
				}
			}

			resourceType := stringsTrim(asString(row["type"]))
			subscriptionID, ok := parseARMResourceID(id)
			if resourceType == "" || !ok {
				continue
			}

			region := stringsLowerTrim(asString(row["location"]))
			if region == "" {
				region = "global"
			}
			if len(regionsFilter) > 0 {
				if _, ok := regionsFilter[region]; !ok {
					continue
				}
			}

			result = append(result, resourceInfo{
				SubscriptionID: subscriptionID,
				ID:             id,
				UID:            hashShort(id),
				Name:           stringsTrim(asString(row["name"])),
				Type:           resourceType,
				ResourceGroup:  rg,
				Region:         region,
				Tags:           normalizeResourceTags(row["tags"]),
			})
		}

		if resp.SkipToken == nil || stringsTrim(*resp.SkipToken) == "" {
			break
		}
		token := stringsTrim(*resp.SkipToken)
		skipToken = &token
	}

	sortResourceInfos(result)
	return result, indexResourcesByType(result), nil
}

func discoverResourcesFromQuery(ctx context.Context, subscriptionIDs []string, timeout time.Duration, resourceGraph resourceGraphClient, kql string, supportedTypes map[string]struct{}) (discoveryFetchResult, error) {
	query := stringsTrim(kql)
	if query == "" {
		return discoveryFetchResult{}, errors.New("custom discovery query is empty")
	}

	result := make([]resourceInfo, 0, 256)
	unsupported := make(map[string]struct{})
	seenIDs := make(map[string]struct{})

	var skipToken *string
	for {
		req := armResourceGraphQuery(subscriptionIDs, query, skipToken)
		reqCtx, cancel := withOptionalTimeout(ctx, timeout)
		resp, err := resourceGraph.Resources(reqCtx, req, nil)
		cancel()
		if err != nil {
			return discoveryFetchResult{}, err
		}

		rows, err := parseResourceGraphObjectArray(resp.Data)
		if err != nil {
			return discoveryFetchResult{}, err
		}

		for i, row := range rows {
			resource, err := parseStrictQueryDiscoveryRow(row)
			if err != nil {
				return discoveryFetchResult{}, fmt.Errorf("query result row %d: %w", i, err)
			}

			idKey := stringsLowerTrim(resource.ID)
			if _, ok := seenIDs[idKey]; ok {
				return discoveryFetchResult{}, fmt.Errorf("query result contains duplicate id %q", resource.ID)
			}
			seenIDs[idKey] = struct{}{}

			result = append(result, resource)
			typeKey := stringsLowerTrim(resource.Type)
			if _, ok := supportedTypes[typeKey]; !ok {
				unsupported[typeKey] = struct{}{}
			}
		}

		if resp.SkipToken == nil || stringsTrim(*resp.SkipToken) == "" {
			break
		}
		token := stringsTrim(*resp.SkipToken)
		skipToken = &token
	}

	sortResourceInfos(result)

	unsupportedTypes := make([]string, 0, len(unsupported))
	for resourceType := range unsupported {
		unsupportedTypes = append(unsupportedTypes, resourceType)
	}
	slices.Sort(unsupportedTypes)

	return discoveryFetchResult{
		Resources:        result,
		ByType:           indexResourcesByType(result),
		UnsupportedTypes: unsupportedTypes,
	}, nil
}

func armResourceGraphQuery(subscriptionIDs []string, query string, skipToken *string) armresourcegraph.QueryRequest {
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

	subs := make([]*string, 0, len(subscriptionIDs))
	for _, subID := range subscriptionIDs {
		subID = stringsTrim(subID)
		if subID == "" {
			continue
		}
		subscription := subID
		subs = append(subs, &subscription)
	}

	return armresourcegraph.QueryRequest{
		Query:         &query,
		Subscriptions: subs,
		Options:       options,
	}
}

func parseARMResourceID(resourceID string) (string, bool) {
	resourceID = stringsTrim(resourceID)
	if resourceID == "" || !strings.HasPrefix(resourceID, "/") {
		return "", false
	}

	parts := strings.Split(strings.Trim(resourceID, "/"), "/")
	if len(parts) < 6 || len(parts)%2 != 0 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "subscriptions") || stringsTrim(parts[1]) == "" {
		return "", false
	}

	hasProviders := false
	for i := 0; i+1 < len(parts); i += 2 {
		if stringsTrim(parts[i]) == "" || stringsTrim(parts[i+1]) == "" {
			return "", false
		}
		if strings.EqualFold(parts[i], "providers") {
			hasProviders = true
		}
	}
	if !hasProviders {
		return "", false
	}

	return stringsTrim(parts[1]), true
}

func buildDiscoveryQuery(resourceTypes []string, filters *ResourceFiltersConfig) string {
	if len(resourceTypes) == 0 {
		return ""
	}

	quotedTypes := make([]string, 0, len(resourceTypes))
	for _, rt := range resourceTypes {
		rt = stringsTrim(rt)
		if rt == "" {
			continue
		}
		if !azureprofiles.IsValidResourceType(rt) {
			continue
		}
		quotedTypes = append(quotedTypes, quoteKQLString(rt))
	}
	if len(quotedTypes) == 0 {
		return ""
	}

	query := "resources | where type in~ (" + strings.Join(quotedTypes, ", ") + ")"

	if filters == nil {
		return query + " | project id, name, type, resourceGroup, location, tags"
	}

	if groups := normalizeFilterValues(filters.ResourceGroups); len(groups) > 0 {
		quotedGroups := make([]string, 0, len(groups))
		for _, rg := range groups {
			quotedGroups = append(quotedGroups, quoteKQLString(rg))
		}
		query += " | where resourceGroup in~ (" + strings.Join(quotedGroups, ", ") + ")"
	}

	if regions := normalizeFilterValues(filters.Regions); len(regions) > 0 {
		quotedRegions := make([]string, 0, len(regions))
		for _, region := range regions {
			quotedRegions = append(quotedRegions, quoteKQLString(region))
		}
		query += " | where location in~ (" + strings.Join(quotedRegions, ", ") + ")"
	}

	if tagFilters := normalizeTagFilters(filters.Tags); len(tagFilters) > 0 {
		query += " | extend tagsBag = tags"
		query += " | mv-expand bagexpansion=array tags"
		query += " | where isnotempty(tags)"
		query += " | extend tagKey = tostring(tags[0]), tagValue = tostring(tags[1])"
		query += " | where " + buildTagPredicate(tagFilters)
		query += " | summarize tags = take_any(tagsBag), matchedTagKeys = dcount(tolower(tagKey)) by id, name, type, resourceGroup, location"
		query += fmt.Sprintf(" | where matchedTagKeys == %d", len(tagFilters))
	}

	return query + " | project id, name, type, resourceGroup, location, tags"
}

func normalizeFilterValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))

	for _, v := range values {
		n := stringsLowerTrim(v)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}

	slices.Sort(out)
	return out
}

func normalizeTagFilters(tags map[string][]string) []normalizedTagFilter {
	if len(tags) == 0 {
		return nil
	}

	out := make([]normalizedTagFilter, 0, len(tags))
	for key, values := range tags {
		normalizedKey := stringsLowerTrim(key)
		if normalizedKey == "" {
			continue
		}

		seenValues := make(map[string]struct{}, len(values))
		normalizedValues := make([]string, 0, len(values))
		for _, value := range values {
			trimmed := stringsTrim(value)
			if trimmed == "" {
				continue
			}
			if _, ok := seenValues[trimmed]; ok {
				continue
			}
			seenValues[trimmed] = struct{}{}
			normalizedValues = append(normalizedValues, trimmed)
		}
		if len(normalizedValues) == 0 {
			continue
		}

		slices.Sort(normalizedValues)
		out = append(out, normalizedTagFilter{Key: normalizedKey, Values: normalizedValues})
	}

	slices.SortFunc(out, func(a, b normalizedTagFilter) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		default:
			return 0
		}
	})
	return out
}

func buildTagPredicate(filters []normalizedTagFilter) string {
	clauses := make([]string, 0, len(filters))
	for _, filter := range filters {
		keyClause := "tagKey =~ " + quoteKQLString(filter.Key)
		valueClause := buildTagValueClause(filter.Values)
		clauses = append(clauses, "("+keyClause+" and "+valueClause+")")
	}
	return strings.Join(clauses, " or ")
}

func buildTagValueClause(values []string) string {
	if len(values) == 1 {
		return "tagValue == " + quoteKQLString(values[0])
	}

	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, quoteKQLString(value))
	}
	return "tagValue in (" + strings.Join(quoted, ", ") + ")"
}

func normalizedFilterSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(values))
	for _, value := range normalizeFilterValues(values) {
		set[value] = struct{}{}
	}
	return set
}

func parseStrictQueryDiscoveryRow(row map[string]any) (resourceInfo, error) {
	id, err := strictQueryStringColumn(row, "id")
	if err != nil {
		return resourceInfo{}, err
	}
	subscriptionID, ok := parseARMResourceID(id)
	if !ok {
		return resourceInfo{}, fmt.Errorf("invalid ARM resource id %q", id)
	}

	name, err := strictQueryStringColumn(row, "name")
	if err != nil {
		return resourceInfo{}, err
	}
	resourceType, err := strictQueryStringColumn(row, "type")
	if err != nil {
		return resourceInfo{}, err
	}
	if resourceType == "" {
		return resourceInfo{}, errors.New("column 'type' must not be empty")
	}

	resourceGroup, err := strictQueryStringColumn(row, "resourceGroup")
	if err != nil {
		return resourceInfo{}, err
	}
	location, err := strictQueryStringColumn(row, "location")
	if err != nil {
		return resourceInfo{}, err
	}
	region := stringsLowerTrim(location)
	if region == "" {
		region = "global"
	}

	return resourceInfo{
		SubscriptionID: subscriptionID,
		ID:             id,
		UID:            hashShort(id),
		Name:           name,
		Type:           resourceType,
		ResourceGroup:  resourceGroup,
		Region:         region,
	}, nil
}

func strictQueryStringColumn(row map[string]any, column string) (string, error) {
	value, ok := row[column]
	if !ok {
		return "", fmt.Errorf("missing required column %q", column)
	}

	switch v := value.(type) {
	case string:
		return stringsTrim(v), nil
	case fmt.Stringer:
		return stringsTrim(v.String()), nil
	default:
		return "", fmt.Errorf("column %q must be a string", column)
	}
}

func filterDiscoveryResourcesByTypes(resources []resourceInfo, allowedTypes []string) ([]resourceInfo, map[string][]resourceInfo) {
	if len(resources) == 0 || len(allowedTypes) == 0 {
		return nil, map[string][]resourceInfo{}
	}

	allowed := make(map[string]struct{}, len(allowedTypes))
	for _, resourceType := range allowedTypes {
		allowed[stringsLowerTrim(resourceType)] = struct{}{}
	}

	filtered := make([]resourceInfo, 0, len(resources))
	for _, resource := range resources {
		typeKey := stringsLowerTrim(resource.Type)
		if _, ok := allowed[typeKey]; !ok {
			continue
		}
		filtered = append(filtered, resource)
	}

	return filtered, indexResourcesByType(filtered)
}

func filterDiscoveryResourcesByProfiles(resources []resourceInfo, runtime *collectorRuntime) map[string][]resourceInfo {
	if runtime == nil || len(runtime.Profiles) == 0 {
		return map[string][]resourceInfo{}
	}

	result := make(map[string][]resourceInfo, len(runtime.Profiles))
	for _, profile := range runtime.Profiles {
		matcher := newProfileResourceMatcher(profile)
		for _, resource := range resources {
			if !matcher.matches(resource) {
				continue
			}
			result[profile.Name] = append(result[profile.Name], resource)
		}
	}
	return result
}

func newProfileResourceMatcher(profile *profileRuntime) profileResourceMatcher {
	matcher := profileResourceMatcher{
		resourceType: stringsLowerTrim(profile.ResourceType),
		profileName:  profile.Name,
	}
	if profile.Filters == nil {
		return matcher
	}

	matcher.resourceGroups = normalizedFilterSet(profile.Filters.ResourceGroups)
	matcher.regions = normalizedFilterSet(profile.Filters.Regions)
	matcher.tagFilters = normalizeTagFilters(profile.Filters.Tags)
	return matcher
}

func (m profileResourceMatcher) matches(resource resourceInfo) bool {
	if stringsLowerTrim(resource.Type) != m.resourceType {
		return false
	}
	if len(m.resourceGroups) > 0 {
		if _, ok := m.resourceGroups[stringsLowerTrim(resource.ResourceGroup)]; !ok {
			return false
		}
	}
	if len(m.regions) > 0 {
		if _, ok := m.regions[normalizeRegion(resource.Region)]; !ok {
			return false
		}
	}
	for _, tagFilter := range m.tagFilters {
		if !resourceMatchesTag(resource, tagFilter) {
			return false
		}
	}
	return true
}

func resourceMatchesTag(resource resourceInfo, filter normalizedTagFilter) bool {
	for _, tag := range resource.Tags {
		if tag.Key != filter.Key {
			continue
		}
		return slices.Contains(filter.Values, tag.Value)
	}
	return false
}

func indexResourcesByType(resources []resourceInfo) map[string][]resourceInfo {
	result := make(map[string][]resourceInfo)
	for _, resource := range resources {
		key := stringsLowerTrim(resource.Type)
		result[key] = append(result[key], resource)
	}
	return result
}

func catalogResourceTypeSet(catalog azureprofiles.Catalog) map[string]struct{} {
	types := make(map[string]struct{})
	for _, resourceType := range catalog.ResourceTypes() {
		types[stringsLowerTrim(resourceType)] = struct{}{}
	}
	return types
}

func quoteKQLString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
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

func normalizeResourceTags(v any) []resourceTag {
	rawTags, ok := v.(map[string]any)
	if !ok {
		if typed, ok := v.(map[string]string); ok {
			rawTags = make(map[string]any, len(typed))
			for key, value := range typed {
				rawTags[key] = value
			}
		} else {
			return nil
		}
	}

	tags := make([]resourceTag, 0, len(rawTags))
	for key, value := range rawTags {
		tagKey := stringsLowerTrim(key)
		if tagKey == "" {
			continue
		}
		tags = append(tags, resourceTag{
			Key:   tagKey,
			Value: normalizeResourceTagValue(value),
		})
	}

	slices.SortFunc(tags, func(a, b resourceTag) int {
		switch {
		case a.Key < b.Key:
			return -1
		case a.Key > b.Key:
			return 1
		case a.Value < b.Value:
			return -1
		case a.Value > b.Value:
			return 1
		default:
			return 0
		}
	})
	return tags
}

func normalizeResourceTagValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return stringsTrim(x)
	case fmt.Stringer:
		return stringsTrim(x.String())
	default:
		return stringsTrim(fmt.Sprint(x))
	}
}

func sortResourceInfos(resources []resourceInfo) {
	slices.SortFunc(resources, func(a, b resourceInfo) int {
		switch {
		case stringsLowerTrim(a.ID) < stringsLowerTrim(b.ID):
			return -1
		case stringsLowerTrim(a.ID) > stringsLowerTrim(b.ID):
			return 1
		default:
			return 0
		}
	})
}

func equalResourceSlices(a, b []resourceInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalResourceInfo(a[i], b[i]) {
			return false
		}
	}
	return true
}

func equalResourceInfo(a, b resourceInfo) bool {
	return a.SubscriptionID == b.SubscriptionID &&
		a.ID == b.ID &&
		a.UID == b.UID &&
		a.Name == b.Name &&
		a.Type == b.Type &&
		a.ResourceGroup == b.ResourceGroup &&
		a.Region == b.Region &&
		equalResourceTags(a.Tags, b.Tags)
}

func equalResourceTags(a, b []resourceTag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
