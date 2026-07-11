// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type tagFetchKey struct {
	target, region, filter string
}

// tagFetchGroup is one deduplicated RGTA request stream. Policy scopes sharing
// target, region, and exact predicate share the fetch while retaining separate
// ordered membership identities.
type tagFetchGroup struct {
	key     tagFetchKey
	account string
	filters []resourceTagFilter

	profiles               map[string]struct{}
	profilesByResourceType map[string][]string
	scopeIDsByProfile      map[string][]int
	candidatesByProfile    map[string]map[string]struct{}
	tagKeys                map[string]struct{}
	resourceTypes          []string
	hasLabels              bool
}

type tagFetchResult struct {
	group           tagFetchGroup
	members         tagMembership
	labels          map[tagCacheKey][]metrix.Label
	confirmedLabels map[tagCacheKey]struct{}
	err             error
}

func (c *Collector) tagFetchGroups() []tagFetchGroup {
	groups := make(map[tagFetchKey]*tagFetchGroup)
	addScope := func(scope collectionScope, filters []resourceTagFilter, includeMembership bool) {
		resolved, ok := c.resolvedTargetByRef(scope.Target.Name)
		if !ok {
			return
		}
		join, ok := tagJoins[scope.Profile.Name]
		if !ok {
			return
		}
		instances := c.discovery.Instances[discoveryKey{Target: scope.Target.Name, Profile: scope.Profile.Name, Region: scope.Region}]
		if len(instances) == 0 {
			return
		}

		filterKey := resourceTagFilterSignature(filters)
		key := tagFetchKey{target: scope.Target.Name, region: scope.Region, filter: filterKey}
		group := groups[key]
		if group == nil {
			group = &tagFetchGroup{
				key: key, account: resolved.accountID, filters: filters,
				profiles:               make(map[string]struct{}),
				profilesByResourceType: make(map[string][]string),
				scopeIDsByProfile:      make(map[string][]int),
				candidatesByProfile:    make(map[string]map[string]struct{}),
				tagKeys:                make(map[string]struct{}),
			}
			for _, filter := range filters {
				group.tagKeys[filter.key] = struct{}{}
			}
			groups[key] = group
		}
		if _, exists := group.profiles[scope.Profile.Name]; !exists {
			group.profiles[scope.Profile.Name] = struct{}{}
			for _, resourceType := range join.resourceTypes {
				group.profilesByResourceType[resourceType] = append(group.profilesByResourceType[resourceType], scope.Profile.Name)
				if !slices.Contains(group.resourceTypes, resourceType) {
					group.resourceTypes = append(group.resourceTypes, resourceType)
				}
			}
			for _, label := range c.tagLabelPlans[scope.Profile.Name] {
				group.tagKeys[label.awsKey] = struct{}{}
				group.hasLabels = true
			}
		}
		if includeMembership && !slices.Contains(group.scopeIDsByProfile[scope.Profile.Name], scope.ID) {
			group.scopeIDsByProfile[scope.Profile.Name] = append(group.scopeIDsByProfile[scope.Profile.Name], scope.ID)
		}
		candidates := group.candidatesByProfile[scope.Profile.Name]
		if candidates == nil {
			candidates = make(map[string]struct{}, len(instances))
			group.candidatesByProfile[scope.Profile.Name] = candidates
		}
		dimNames := scope.Profile.Config.DimensionNames()
		for _, instance := range instances {
			if joinKey, ok := join.instanceJoinKey(dimNames, instance.DimensionValues); ok {
				candidates[joinKey] = struct{}{}
			}
		}
	}

	for _, scope := range c.plan.Scopes {
		if scope.hasTagFilter() {
			addScope(scope, scope.TagFilter, true)
		}
		if len(c.tagLabelPlans[scope.Profile.Name]) > 0 && !scope.hasTagFilter() {
			addScope(scope, nil, false)
		}
	}

	out := make([]tagFetchGroup, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group.resourceTypes)
		for resourceType := range group.profilesByResourceType {
			sort.Strings(group.profilesByResourceType[resourceType])
		}
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].key, out[j].key
		if a.target != b.target {
			return a.target < b.target
		}
		if a.region != b.region {
			return a.region < b.region
		}
		return a.filter < b.filter
	})
	return out
}

func (c *Collector) fetchTagGroup(ctx context.Context, group tagFetchGroup) tagFetchResult {
	result := tagFetchResult{
		group:           group,
		members:         make(tagMembership),
		labels:          make(map[tagCacheKey][]metrix.Label),
		confirmedLabels: make(map[tagCacheKey]struct{}),
	}
	client, err := c.rgtaClients.forTargetRegion(ctx, group.key.target, group.key.region)
	if err != nil {
		result.err = fmt.Errorf("build client: %w", err)
		return result
	}
	result.err = walkResourceTags(ctx, client, group.resourceTypes, group.filters, c.Timeout.Duration(), func(resource rgtatypes.ResourceTagMapping) {
		indexFetchedResource(result.members, result.labels, result.confirmedLabels, group, resource, c.tagLabelPlans)
	})
	return result
}

func walkResourceTags(ctx context.Context, client rgtaClient, resourceTypes []string, filters []resourceTagFilter, timeout time.Duration, fn func(rgtatypes.ResourceTagMapping)) error {
	cctx, cancel := withTimeout(ctx, timeout)
	defer cancel()

	tagFilters := make([]rgtatypes.TagFilter, len(filters))
	for i, filter := range filters {
		tagFilters[i] = rgtatypes.TagFilter{Key: aws.String(filter.key), Values: slices.Clone(filter.values)}
	}
	seenTokens := make(map[string]struct{})
	var token *string
	for {
		input := &resourcegroupstaggingapi.GetResourcesInput{
			PaginationToken:     token,
			ResourcesPerPage:    aws.Int32(100),
			ResourceTypeFilters: resourceTypes,
			TagFilters:          tagFilters,
		}
		response, err := client.GetResources(cctx, input)
		if err != nil {
			return err
		}
		for _, resource := range response.ResourceTagMappingList {
			fn(resource)
		}
		if response.PaginationToken == nil || *response.PaginationToken == "" {
			return nil
		}
		if _, duplicate := seenTokens[*response.PaginationToken]; duplicate {
			return fmt.Errorf("GetResources returned duplicate pagination token")
		}
		seenTokens[*response.PaginationToken] = struct{}{}
		token = response.PaginationToken
	}
}

func indexFetchedResource(
	members tagMembership,
	labels map[tagCacheKey][]metrix.Label,
	confirmedLabels map[tagCacheKey]struct{},
	group tagFetchGroup,
	resource rgtatypes.ResourceTagMapping,
	labelPlans map[string][]resolvedTag,
) {
	if resource.ResourceARN == nil {
		return
	}
	parsed, err := arn.Parse(*resource.ResourceARN)
	if err != nil {
		return
	}
	if (parsed.AccountID != "" && parsed.AccountID != group.account) ||
		(parsed.Region != "" && parsed.Region != group.key.region) {
		return
	}
	profileNames := group.profilesByResourceType[deriveResourceType(parsed)]
	if len(profileNames) == 0 {
		return
	}
	tags := selectedTagMap(resource.Tags, group.tagKeys)
	if !resourceMatchesFilters(tags, group.filters) {
		return
	}
	for _, profileName := range profileNames {
		join := tagJoins[profileName]
		joinKey, ok := join.arnJoinKey(parsed)
		if !ok {
			continue
		}
		if _, candidate := group.candidatesByProfile[profileName][joinKey]; !candidate {
			continue
		}
		for _, scopeID := range group.scopeIDsByProfile[profileName] {
			members.add(scopeID, joinKey)
		}
		plan := labelPlans[profileName]
		if len(plan) == 0 {
			continue
		}
		labelKey := tagCacheKey{
			target: group.key.target, account: group.account, region: group.key.region,
			profile: profileName, joinKey: joinKey,
		}
		confirmedLabels[labelKey] = struct{}{}
		if resolved := applyTagPlan(plan, tags); len(resolved) > 0 {
			labels[labelKey] = resolved
		}
	}
}
