// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

func TestPrivateLinkEndpointProfiles_PublicContract(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	type metricContract struct {
		statistics []string
		rate       bool
	}

	tests := map[string]struct {
		disabled   bool
		dimensions []cwprofiles.InstanceDimension
		byLabels   []string
		chartIDs   []string
	}{
		"privatelink_endpoint": {
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Endpoint Type", Label: "endpoint_type"},
				{Name: "Service Name", Label: "service_name"},
				{Name: "VPC Endpoint Id", Label: "vpc_endpoint_id"},
				{Name: "VPC Id", Label: "vpc_id"},
			},
			byLabels: []string{"account_id", "region", "endpoint_type", "service_name", "vpc_endpoint_id", "vpc_id"},
			chartIDs: []string{
				"aws_cloudwatch_privatelink_endpoint_active_connections",
				"aws_cloudwatch_privatelink_endpoint_average_processed_bytes",
				"aws_cloudwatch_privatelink_endpoint_processed_bytes",
				"aws_cloudwatch_privatelink_endpoint_average_new_connections",
				"aws_cloudwatch_privatelink_endpoint_new_connections",
				"aws_cloudwatch_privatelink_endpoint_packet_problems",
			},
		},
		"privatelink_endpoint_subnet": {
			disabled: true,
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Endpoint Type", Label: "endpoint_type"},
				{Name: "Service Name", Label: "service_name"},
				{Name: "Subnet Id", Label: "subnet_id"},
				{Name: "VPC Endpoint Id", Label: "vpc_endpoint_id"},
				{Name: "VPC Id", Label: "vpc_id"},
			},
			byLabels: []string{"account_id", "region", "endpoint_type", "service_name", "subnet_id", "vpc_endpoint_id", "vpc_id"},
			chartIDs: []string{
				"aws_cloudwatch_privatelink_endpoint_subnet_active_connections",
				"aws_cloudwatch_privatelink_endpoint_subnet_average_processed_bytes",
				"aws_cloudwatch_privatelink_endpoint_subnet_processed_bytes",
				"aws_cloudwatch_privatelink_endpoint_subnet_average_new_connections",
				"aws_cloudwatch_privatelink_endpoint_subnet_new_connections",
				"aws_cloudwatch_privatelink_endpoint_subnet_packet_problems",
			},
		},
	}

	wantMetrics := map[string]metricContract{
		"ActiveConnections":  {statistics: []string{"average"}},
		"BytesProcessed":     {statistics: []string{"average", "sum"}, rate: true},
		"NewConnections":     {statistics: []string{"average", "sum"}, rate: true},
		"PacketsDropped":     {statistics: []string{"sum"}, rate: true},
		"RstPacketsReceived": {statistics: []string{"sum"}, rate: true},
	}

	for profileName, tc := range tests {
		t.Run(profileName, func(t *testing.T) {
			profile, ok := catalog.Get(profileName)
			require.True(t, ok)
			assert.Equal(t, "AWS/PrivateLinkEndpoints", profile.Namespace)
			assert.Equal(t, tc.disabled, profile.Disabled)
			assert.Equal(t, tc.dimensions, profile.Instance.Dimensions)
			require.NotNil(t, profile.Query.Period)
			require.NotNil(t, profile.Query.Lookback)
			require.NotNil(t, profile.Query.PublicationDelay)
			assert.Equal(t, 5*time.Minute, profile.Query.Period.Duration())
			assert.Equal(t, 5*time.Minute, profile.Query.Lookback.Duration())
			assert.Equal(t, 5*time.Minute, profile.Query.PublicationDelay.Duration())

			gotMetrics := make(map[string]metricContract, len(profile.Metrics))
			for _, metric := range profile.Metrics {
				gotMetrics[metric.MetricName] = metricContract{statistics: metric.Statistics, rate: metric.Rate}
			}
			assert.Equal(t, wantMetrics, gotMetrics)

			assert.Equal(t, "PrivateLink", profile.Template.Family)
			assert.Equal(t, profileName, profile.Template.ContextNamespace)
			require.NotNil(t, profile.Template.ChartDefaults)
			require.NotNil(t, profile.Template.ChartDefaults.Instances)
			assert.Equal(t, tc.byLabels, profile.Template.ChartDefaults.Instances.ByLabels)
			require.Len(t, profile.Template.Charts, len(tc.chartIDs))
			for i, chart := range profile.Template.Charts {
				assert.Equal(t, tc.chartIDs[i], chart.ID)
				assert.Equal(t, "absolute", chart.Algorithm)
			}
		})
	}
}

func TestPrivateLinkEndpointProfiles_ShareDiscoveryAndMatchExactGrains(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{
		Defaults: &defaults, Include: []string{"privatelink_endpoint", "privatelink_endpoint_subnet"},
	}
	cfg.applyDefaults()
	plan, diagnostics, err := compileConfig(cfg, catalog)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	c := New()
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{
		"base": {target: plan.Targets[0], accountID: "000000000000"},
	}
	groups := c.discoveryGroups()
	require.Len(t, groups, 1, "both exact grains share one namespace scan")
	assert.Equal(t, "AWS/PrivateLinkEndpoints", groups[0].Namespace)
	assert.True(t, groups[0].RecentlyActive)
	assert.ElementsMatch(t, []string{"privatelink_endpoint", "privatelink_endpoint_subnet"}, profileNames(groups[0].Profiles))

	listed := []cwtypes.Metric{
		mkMetric("ActiveConnections", "VPC Id", "vpc-1", "VPC Endpoint Id", "vpce-1", "Service Name", "service-1", "Endpoint Type", "Interface"),
		mkMetric("BytesProcessed", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
		mkMetric("NewConnections", "Endpoint Type", "Interface", "Service Name", "service-1", "Subnet Id", "subnet-a", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
		mkMetric("PacketsDropped", "Endpoint Type", "Interface", "Service Name", "service-1", "Subnet Id", "subnet-b", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
		mkMetric("RstPacketsReceived", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1", "Availability Zone", "us-east-1a"),
		mkMetric("EndpointsCount", "Endpoint Type", "Interface", "Service Name", "service-1", "VPC Endpoint Id", "vpce-1", "VPC Id", "vpc-1"),
	}

	instances, err := scanDiscoveryGroupForTest(context.Background(), &fakeCloudWatch{
		pages: []*cloudwatch.ListMetricsOutput{page(listed, "")},
	}, groups[0])
	require.NoError(t, err)
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"Interface", "service-1", "vpce-1", "vpc-1"}}}, instances["privatelink_endpoint"])
	assert.ElementsMatch(t, []collectionInstance{
		{DimensionValues: []string{"Interface", "service-1", "subnet-a", "vpce-1", "vpc-1"}},
		{DimensionValues: []string{"Interface", "service-1", "subnet-b", "vpce-1", "vpc-1"}},
	}, instances["privatelink_endpoint_subnet"])
}

func TestPrivateLinkEndpointProfiles_ExactRulePoliciesRemainDisjoint(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{
		{
			Name: "one-minute-averages", Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"privatelink_endpoint"}},
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "privatelink_endpoint", Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "ActiveConnections"}, {Name: "BytesProcessed"}, {Name: "NewConnections"}},
			}},
			Query: &cwquery.Config{Period: longDuration(time.Minute), Lookback: longDuration(5 * time.Minute), PublicationDelay: longDuration(5 * time.Minute)},
		},
		{
			Name: "six-hour-bytes", Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"privatelink_endpoint"}},
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "privatelink_endpoint",
				Include: []MetricSelectionConfig{{Name: "BytesProcessed", Statistics: []string{"Sum"}}},
			}},
			Query: &cwquery.Config{Period: longDuration(6 * time.Hour), Lookback: longDuration(6 * time.Hour), PublicationDelay: longDuration(5 * time.Minute)},
		},
	}
	cfg.applyDefaults()

	plan, diagnostics, err := compileConfig(cfg, catalog)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	require.Len(t, plan.Scopes, 2)
	policies := make(map[string]cwquery.Policy)
	for _, scope := range plan.Scopes {
		for _, series := range scope.SelectedSeries {
			policies[series.Name] = series.Policy
		}
	}
	assert.Equal(t, map[string]cwquery.Policy{
		"privatelink_endpoint.active_connections_average": {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_endpoint.bytes_processed_average":    {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_endpoint.new_connections_average":    {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_endpoint.bytes_processed_sum":        {Period: 6 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 5 * time.Minute},
	}, policies)

	c := New()
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{
		"base": {target: plan.Targets[0], accountID: "000000000000"},
	}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]collectionInstance{
		{Target: "base", Profile: "privatelink_endpoint", Region: "us-east-1"}: {
			{DimensionValues: []string{"Interface", "service-1", "vpce-1", "vpc-1"}},
		},
	}}
	queries, err := c.buildQueryPlan()
	require.NoError(t, err)
	gotRate := make(map[string]bool, len(queries))
	for _, query := range queries {
		gotRate[query.seriesName] = query.rate
	}
	assert.Equal(t, map[string]bool{
		"privatelink_endpoint.active_connections_average": false,
		"privatelink_endpoint.bytes_processed_average":    false,
		"privatelink_endpoint.new_connections_average":    false,
		"privatelink_endpoint.bytes_processed_sum":        true,
	}, gotRate, "rate normalization applies only to the Sum sibling")
}

func TestPrivateLinkEndpointProfiles_ResourceTagsJoinParentEndpoint(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	filters := []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg := validBaseConfig()
	cfg.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{
		Defaults: &defaults, Include: []string{"privatelink_endpoint", "privatelink_endpoint_subnet"},
	}
	cfg.Rules[0].Filters = &RuleFiltersConfig{ResourceTags: &filters}
	cfg.applyDefaults()

	plan, diagnostics, err := compileConfig(cfg, catalog)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	c := New()
	c.Config = cfg
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{
		"base": {target: plan.Targets[0], accountID: "000000000000"},
	}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]collectionInstance{
		{Target: "base", Profile: "privatelink_endpoint", Region: "us-east-1"}: {
			{DimensionValues: []string{"Interface", "service-1", "vpce-1", "vpc-1"}},
		},
		{Target: "base", Profile: "privatelink_endpoint_subnet", Region: "us-east-1"}: {
			{DimensionValues: []string{"Interface", "service-1", "subnet-a", "vpce-1", "vpc-1"}},
			{DimensionValues: []string{"Interface", "service-1", "subnet-b", "vpce-1", "vpc-1"}},
		},
	}}
	c.computeTagLabelPlans()

	groups := c.currentTagFetchPlan()
	require.Len(t, groups, 1, "both profiles share one VPC endpoint RGTA request")
	group := groups[0]
	assert.Equal(t, []string{"ec2:vpc-endpoint"}, group.resourceTypes)
	assert.ElementsMatch(t, []string{"privatelink_endpoint", "privatelink_endpoint_subnet"}, group.profilesByResourceType["ec2:vpc-endpoint"])
	assert.Equal(t, map[string]struct{}{"vpce-1": {}}, group.candidatesByProfile["privatelink_endpoint"])
	assert.Equal(t, map[string]struct{}{"vpce-1": {}}, group.candidatesByProfile["privatelink_endpoint_subnet"])

	members := make(tagMembership)
	labels := make(map[tagCacheKey][]metrix.Label)
	confirmed := make(map[tagCacheKey]struct{})
	indexFetchedResource(members, labels, confirmed, group,
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:vpc-endpoint/vpce-1", "environment", "production", "owner", "platform"),
		c.tagLabelPlans,
	)
	for _, profileName := range []string{"privatelink_endpoint", "privatelink_endpoint_subnet"} {
		membershipID, ok := group.membershipIDByProfile[profileName]
		require.True(t, ok)
		assert.Contains(t, members[membershipID], "vpce-1")
		assert.Equal(t, []metrix.Label{{Key: "owner", Value: "platform"}}, labels[tagCacheKey{
			target: "base", account: "000000000000", region: "us-east-1", profile: profileName, joinKey: "vpce-1",
		}])
	}
	c.tags.labels = labels
	for _, scope := range c.plan.Scopes {
		for _, instance := range c.discovery.Instances[discoveryKey{Target: "base", Profile: scope.Profile.Name, Region: "us-east-1"}] {
			assert.Equal(t, []metrix.Label{{Key: "owner", Value: "platform"}}, c.tagLabelsFor(
				"base", "000000000000", "us-east-1", scope.Profile, c.plan.TagJoins[scope.Profile.Name], instance.DimensionValues,
			), "every subnet child inherits the parent endpoint label")
		}
	}
}
