// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
)

func privateLinkServiceChartContracts(profileName string, includeEndpoints bool) []privateLinkChartContract {
	prefix := "aws_cloudwatch_" + profileName + "_"
	charts := []privateLinkChartContract{
		{
			id: prefix + "active_connections", context: "active_connections", title: "PrivateLink Service Active Connections",
			family: "Connections", units: "connections",
			dimensions: []privateLinkChartDimensionContract{{selector: "active_connections_average", name: "active"}},
		},
		{
			id: prefix + "average_processed_bytes", context: "average_processed_bytes", title: "PrivateLink Service Average Processed Bytes",
			family: "Traffic", units: "bytes",
			dimensions: []privateLinkChartDimensionContract{{selector: "bytes_processed_average", name: "average"}},
		},
		{
			id: prefix + "processed_bytes", context: "processed_bytes", title: "PrivateLink Service Processed Bytes",
			family: "Traffic", units: "bytes/s",
			dimensions: []privateLinkChartDimensionContract{{selector: "bytes_processed_sum", name: "processed"}},
		},
	}
	if includeEndpoints {
		charts = append(charts, privateLinkChartContract{
			id: prefix + "connected_endpoints", context: "connected_endpoints", title: "PrivateLink Service Connected Endpoints",
			family: "Endpoints", units: "endpoints",
			dimensions: []privateLinkChartDimensionContract{{selector: "endpoints_count_average", name: "connected"}},
		})
	}
	return append(charts,
		privateLinkChartContract{
			id: prefix + "average_new_connections", context: "average_new_connections", title: "PrivateLink Service Average New Connections",
			family: "Connections", units: "connections",
			dimensions: []privateLinkChartDimensionContract{{selector: "new_connections_average", name: "average"}},
		},
		privateLinkChartContract{
			id: prefix + "new_connections", context: "new_connections", title: "PrivateLink Service New Connections",
			family: "Connections", units: "connections/s",
			dimensions: []privateLinkChartDimensionContract{{selector: "new_connections_sum", name: "new"}},
		},
		privateLinkChartContract{
			id: prefix + "average_reset_packets_sent", context: "average_reset_packets_sent", title: "PrivateLink Service Average Reset Packets Sent",
			family: "Packets", units: "packets",
			dimensions: []privateLinkChartDimensionContract{{selector: "rst_packets_sent_average", name: "average"}},
		},
		privateLinkChartContract{
			id: prefix + "reset_packets_sent", context: "reset_packets_sent", title: "PrivateLink Service Reset Packets Sent",
			family: "Packets", units: "packets/s",
			dimensions: []privateLinkChartDimensionContract{{selector: "rst_packets_sent_sum", name: "reset_sent"}},
		},
	)
}

func TestPrivateLinkServiceProfiles_PublicContract(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	type metricContract struct {
		id         string
		statistics []string
		rate       bool
	}
	tests := map[string]struct {
		disabled         bool
		dimensions       []cwprofiles.InstanceDimension
		byLabels         []string
		includeEndpoints bool
	}{
		"privatelink_service": {
			dimensions:       []cwprofiles.InstanceDimension{{Name: "Service Id", Label: "service_id"}},
			byLabels:         []string{"account_id", "region", "service_id"},
			includeEndpoints: true,
		},
		"privatelink_service_az": {
			disabled: true,
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Az", Label: "availability_zone"},
				{Name: "Service Id", Label: "service_id"},
			},
			byLabels: []string{"account_id", "region", "availability_zone", "service_id"},
		},
		"privatelink_service_load_balancer": {
			disabled: true,
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Load Balancer Arn", Label: "load_balancer_arn"},
				{Name: "Service Id", Label: "service_id"},
			},
			byLabels: []string{"account_id", "region", "load_balancer_arn", "service_id"},
		},
		"privatelink_service_az_load_balancer": {
			disabled: true,
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Az", Label: "availability_zone"},
				{Name: "Load Balancer Arn", Label: "load_balancer_arn"},
				{Name: "Service Id", Label: "service_id"},
			},
			byLabels: []string{"account_id", "region", "availability_zone", "load_balancer_arn", "service_id"},
		},
		"privatelink_service_vpc_endpoint": {
			disabled: true,
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Service Id", Label: "service_id"},
				{Name: "VPC Endpoint Id", Label: "vpc_endpoint_id"},
			},
			byLabels: []string{"account_id", "region", "service_id", "vpc_endpoint_id"},
		},
	}

	wantTraffic := map[string]metricContract{
		"ActiveConnections": {id: "active_connections", statistics: []string{"average"}},
		"BytesProcessed":    {id: "bytes_processed", statistics: []string{"average", "sum"}, rate: true},
		"NewConnections":    {id: "new_connections", statistics: []string{"average", "sum"}, rate: true},
		"RstPacketsSent":    {id: "rst_packets_sent", statistics: []string{"average", "sum"}, rate: true},
	}

	for profileName, tc := range tests {
		t.Run(profileName, func(t *testing.T) {
			profile, ok := catalog.Get(profileName)
			require.True(t, ok)
			assert.Equal(t, "AWS/PrivateLinkServices", profile.Namespace)
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
				gotMetrics[metric.MetricName] = metricContract{id: metric.ID, statistics: metric.Statistics, rate: metric.Rate}
			}
			wantMetrics := make(map[string]metricContract, len(wantTraffic)+1)
			maps.Copy(wantMetrics, wantTraffic)
			if tc.includeEndpoints {
				wantMetrics["EndpointsCount"] = metricContract{id: "endpoints_count", statistics: []string{"average"}}
			}
			assert.Equal(t, wantMetrics, gotMetrics)

			var endpointsMetric *cwprofiles.Metric
			for i := range profile.Metrics {
				if profile.Metrics[i].MetricName == "EndpointsCount" {
					endpointsMetric = &profile.Metrics[i]
					break
				}
			}
			assert.Equal(t, tc.includeEndpoints, endpointsMetric != nil)
			if endpointsMetric != nil {
				assert.True(t, endpointsMetric.EmitZeroOnNoData("average"))
			}

			assert.Equal(t, "PrivateLink", profile.Template.Family)
			assert.Equal(t, profileName, profile.Template.ContextNamespace)
			require.NotNil(t, profile.Template.ChartDefaults)
			require.NotNil(t, profile.Template.ChartDefaults.Instances)
			assert.Equal(t, tc.byLabels, profile.Template.ChartDefaults.Instances.ByLabels)
			wantCharts := privateLinkServiceChartContracts(profileName, tc.includeEndpoints)
			require.Len(t, profile.Template.Charts, len(wantCharts))
			for i, want := range wantCharts {
				chart := profile.Template.Charts[i]
				assert.Equal(t, want.id, chart.ID)
				assert.Equal(t, want.context, chart.Context)
				assert.Equal(t, want.title, chart.Title)
				assert.Equal(t, want.family, chart.Family)
				assert.Equal(t, want.units, chart.Units)
				assert.Equal(t, "absolute", chart.Algorithm)
				require.Len(t, chart.Dimensions, len(want.dimensions))
				for j, dimension := range want.dimensions {
					assert.Equal(t, profileName+"."+dimension.selector, chart.Dimensions[j].Selector)
					assert.Equal(t, dimension.name, chart.Dimensions[j].Name)
				}
			}
		})
	}
}

func TestPrivateLinkServiceProfiles_ShareDiscoveryAndMatchExactGrains(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	profiles := []string{
		"privatelink_service",
		"privatelink_service_az",
		"privatelink_service_load_balancer",
		"privatelink_service_az_load_balancer",
		"privatelink_service_vpc_endpoint",
	}
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: profiles}
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
	require.Len(t, groups, 1, "all exact grains share one namespace scan")
	assert.Equal(t, "AWS/PrivateLinkServices", groups[0].Namespace)
	assert.True(t, groups[0].RecentlyActive)
	assert.ElementsMatch(t, profiles, profileNames(groups[0].Profiles))

	listed := []cwtypes.Metric{
		mkMetric("EndpointsCount", "Service Id", "vpce-svc-1"),
		mkMetric("ActiveConnections", "Az", "us-east-1a", "Service Id", "vpce-svc-1"),
		mkMetric("BytesProcessed", "Load Balancer Arn", "net/lb/hash", "Service Id", "vpce-svc-1"),
		mkMetric("NewConnections", "Az", "us-east-1a", "Load Balancer Arn", "net/lb/hash", "Service Id", "vpce-svc-1"),
		mkMetric("RstPacketsSent", "Service Id", "vpce-svc-1", "VPC Endpoint Id", "vpce-1"),
	}

	instances, err := scanDiscoveryGroupForTest(context.Background(), &fakeCloudWatch{
		pages: []*cloudwatch.ListMetricsOutput{page(listed, "")},
	}, groups[0])
	require.NoError(t, err)
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"vpce-svc-1"}}}, instances["privatelink_service"])
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"us-east-1a", "vpce-svc-1"}}}, instances["privatelink_service_az"])
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"net/lb/hash", "vpce-svc-1"}}}, instances["privatelink_service_load_balancer"])
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"us-east-1a", "net/lb/hash", "vpce-svc-1"}}}, instances["privatelink_service_az_load_balancer"])
	assert.Equal(t, []collectionInstance{{DimensionValues: []string{"vpce-svc-1", "vpce-1"}}}, instances["privatelink_service_vpc_endpoint"])
}

func TestPrivateLinkServiceProfiles_ExactRulePoliciesRemainDisjoint(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{
		{
			Name: "one-minute-traffic", Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"privatelink_service"}},
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "privatelink_service", Defaults: &defaults, Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "ActiveConnections"}, {Name: "BytesProcessed"}, {Name: "NewConnections"}, {Name: "RstPacketsSent"}},
			}},
			Query: &cwquery.Config{Period: longDuration(time.Minute), Lookback: longDuration(5 * time.Minute), PublicationDelay: longDuration(5 * time.Minute)},
		},
		{
			Name: "five-minute-endpoints", Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"privatelink_service"}},
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "privatelink_service", Defaults: &defaults,
				Include: []MetricSelectionConfig{{Name: "EndpointsCount", Statistics: []string{"Average"}}},
			}},
			Query: &cwquery.Config{Period: longDuration(5 * time.Minute), Lookback: longDuration(5 * time.Minute), PublicationDelay: longDuration(5 * time.Minute)},
		},
		{
			Name: "six-hour-bytes", Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"privatelink_service"}},
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "privatelink_service", Defaults: &defaults,
				Include: []MetricSelectionConfig{{Name: "BytesProcessed", Statistics: []string{"Sum"}}},
			}},
			Query: &cwquery.Config{Period: longDuration(6 * time.Hour), Lookback: longDuration(6 * time.Hour), PublicationDelay: longDuration(5 * time.Minute)},
		},
	}
	cfg.applyDefaults()

	plan, diagnostics, err := compileConfig(cfg, catalog)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	require.Len(t, plan.Scopes, 3)
	policies := make(map[string]cwquery.Policy)
	for _, scope := range plan.Scopes {
		for _, series := range scope.SelectedSeries {
			policies[series.Name] = series.Policy
		}
	}
	assert.Equal(t, map[string]cwquery.Policy{
		"privatelink_service.active_connections_average": {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_service.bytes_processed_average":    {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_service.new_connections_average":    {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_service.rst_packets_sent_average":   {Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_service.endpoints_count_average":    {Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		"privatelink_service.bytes_processed_sum":        {Period: 6 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 5 * time.Minute},
	}, policies)
}

func TestPrivateLinkServiceProfiles_ResourceTagsJoinParentService(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	profiles := []string{
		"privatelink_service",
		"privatelink_service_az",
		"privatelink_service_load_balancer",
		"privatelink_service_az_load_balancer",
		"privatelink_service_vpc_endpoint",
	}
	defaults := false
	filters := []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg := validBaseConfig()
	cfg.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: profiles}
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
		{Target: "base", Profile: "privatelink_service", Region: "us-east-1"}: {
			{DimensionValues: []string{"vpce-svc-1"}},
		},
		{Target: "base", Profile: "privatelink_service_az", Region: "us-east-1"}: {
			{DimensionValues: []string{"us-east-1a", "vpce-svc-1"}},
		},
		{Target: "base", Profile: "privatelink_service_load_balancer", Region: "us-east-1"}: {
			{DimensionValues: []string{"net/lb/hash", "vpce-svc-1"}},
		},
		{Target: "base", Profile: "privatelink_service_az_load_balancer", Region: "us-east-1"}: {
			{DimensionValues: []string{"us-east-1a", "net/lb/hash", "vpce-svc-1"}},
		},
		{Target: "base", Profile: "privatelink_service_vpc_endpoint", Region: "us-east-1"}: {
			{DimensionValues: []string{"vpce-svc-1", "vpce-1"}},
		},
	}}
	c.computeTagLabelPlans()

	groups := c.currentTagFetchPlan()
	require.Len(t, groups, 1, "all service profiles share one endpoint-service RGTA request")
	group := groups[0]
	assert.Equal(t, []string{"ec2:vpc-endpoint-service"}, group.resourceTypes)
	assert.ElementsMatch(t, profiles, group.profilesByResourceType["ec2:vpc-endpoint-service"])
	for _, profileName := range profiles {
		assert.Equal(t, map[string]struct{}{"vpce-svc-1": {}}, group.candidatesByProfile[profileName])
	}

	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:vpc-endpoint-service/vpce-svc-1", "environment", "production", "owner", "platform"),
	}}
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	c.refreshTags(context.Background())
	require.Equal(t, 1, rgta.calls)
	assert.Equal(t, []string{"ec2:vpc-endpoint-service"}, rgta.gotFilters)

	for _, profileName := range profiles {
		membershipID, ok := group.membershipIDByProfile[profileName]
		require.True(t, ok)
		assert.True(t, c.tags.membershipSelected(membershipID, "vpce-svc-1"))
		assert.Equal(t, []metrix.Label{{Key: "owner", Value: "platform"}}, c.tags.labels[tagCacheKey{
			target: "base", account: "000000000000", region: "us-east-1", profile: profileName, joinKey: "vpce-svc-1",
		}])
	}
	for _, scope := range c.plan.Scopes {
		for _, instance := range c.discovery.Instances[discoveryKey{Target: "base", Profile: scope.Profile.Name, Region: "us-east-1"}] {
			assert.Equal(t, []metrix.Label{{Key: "owner", Value: "platform"}}, c.tagLabelsFor(
				"base", "000000000000", "us-east-1", scope.Profile, c.plan.TagJoins[scope.Profile.Name], instance.DimensionValues,
			), "every detailed child inherits the parent service label")
		}
	}
	queries, err := c.buildQueryPlan()
	require.NoError(t, err)
	require.Len(t, queries, 36, "one service instance exports eight series; four detailed children export seven each")
	for _, query := range queries {
		assert.Equal(t, "platform", labelValue(query.tagLabels, "owner"), query.seriesName)
	}
}
