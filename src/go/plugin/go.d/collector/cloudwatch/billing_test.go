// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func billingTestCollector(t testing.TB, profileNames ...string) *Collector {
	t.Helper()

	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: profileNames}
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
	c.invalidateQueryPlan()
	return c
}

func TestBillingProfiles_CompileStaticAndDynamicGrains(t *testing.T) {
	c := billingTestCollector(t,
		"billing_total",
		"billing_service",
		"billing_linked_account",
		"billing_linked_account_service",
	)

	require.Len(t, c.plan.Scopes, 4)
	staticScopes := 0
	for _, scope := range c.plan.Scopes {
		if scope.StaticInstance == nil {
			continue
		}
		staticScopes++
		assert.Equal(t, "billing_total", scope.Profile.Name)
		assert.Equal(t, []string{"USD"}, scope.StaticInstance.DimensionValues)
	}
	assert.Equal(t, 1, staticScopes)

	groups := c.discoveryGroups()
	require.Len(t, groups, 1, "the three dynamic grains share one namespace scan")
	assert.Equal(t, "AWS/Billing", groups[0].Namespace)
	assert.Equal(t, "us-east-1", groups[0].Region)
	assert.False(t, groups[0].RecentlyActive, "the 24h retrieval horizon must not request PT3H-only discovery")
	assert.ElementsMatch(t, []string{
		"billing_service",
		"billing_linked_account",
		"billing_linked_account_service",
	}, profileNames(groups[0].Profiles))
}

func TestBillingProfiles_PublicContract(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	tests := map[string]struct {
		dimensions []cwprofiles.InstanceDimension
		byLabels   []string
		title      string
	}{
		"billing_total": {
			dimensions: []cwprofiles.InstanceDimension{{Name: "Currency", Constant: aws.String("USD")}},
			byLabels:   []string{"account_id", "region"},
			title:      "Latest AWS Estimated Charges (Worldwide MTD)",
		},
		"billing_service": {
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Currency", Constant: aws.String("USD")},
				{Name: "ServiceName", Label: "service_name"},
			},
			byLabels: []string{"account_id", "region", "service_name"},
			title:    "Latest AWS Estimated Charges by Service (Worldwide MTD)",
		},
		"billing_linked_account": {
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Currency", Constant: aws.String("USD")},
				{Name: "LinkedAccount", Label: "linked_account_id"},
			},
			byLabels: []string{"account_id", "region", "linked_account_id"},
			title:    "Latest AWS Estimated Charges by Linked Account (Worldwide MTD)",
		},
		"billing_linked_account_service": {
			dimensions: []cwprofiles.InstanceDimension{
				{Name: "Currency", Constant: aws.String("USD")},
				{Name: "LinkedAccount", Label: "linked_account_id"},
				{Name: "ServiceName", Label: "service_name"},
			},
			byLabels: []string{"account_id", "region", "linked_account_id", "service_name"},
			title:    "Latest AWS Estimated Charges by Account/Service (Worldwide MTD)",
		},
	}

	for profileName, tc := range tests {
		t.Run(profileName, func(t *testing.T) {
			profile, ok := catalog.Get(profileName)
			require.True(t, ok)
			assert.Equal(t, "AWS/Billing", profile.Namespace)
			assert.Equal(t, []string{"us-east-1"}, profile.SupportedRegions)
			assert.True(t, profile.Disabled)
			assert.Equal(t, tc.dimensions, profile.Instance.Dimensions)
			require.Len(t, profile.Metrics, 1)
			assert.Equal(t, "estimated_charges", profile.Metrics[0].ID)
			assert.Equal(t, "EstimatedCharges", profile.Metrics[0].MetricName)
			assert.Equal(t, []string{"maximum"}, profile.Metrics[0].Statistics)
			assert.Equal(t, "Billing", profile.Template.Family)
			assert.Equal(t, profileName, profile.Template.ContextNamespace)
			require.NotNil(t, profile.Template.ChartDefaults)
			require.NotNil(t, profile.Template.ChartDefaults.Instances)
			assert.Equal(t, tc.byLabels, profile.Template.ChartDefaults.Instances.ByLabels)
			require.Len(t, profile.Template.Charts, 1)
			chart := profile.Template.Charts[0]
			assert.Equal(t, "aws_cloudwatch_"+profileName+"_estimated_charges", chart.ID)
			assert.Equal(t, "estimated_charges", chart.Context)
			assert.Equal(t, tc.title, chart.Title)
			assert.Equal(t, "USD", chart.Units)
			assert.Equal(t, "absolute", chart.Algorithm)
			require.Len(t, chart.Dimensions, 1)
			assert.Equal(t, "estimated_charges", chart.Dimensions[0].Name)
		})
	}
}

func TestBillingProfiles_DiscoveryMatchesExactDimensionGrain(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	listed := []cwtypes.Metric{
		mkMetric("EstimatedCharges", "Currency", "USD"),
		mkMetric("EstimatedCharges", "Currency", "USD", "ServiceName", "Compute"),
		mkMetric("EstimatedCharges", "Currency", "USD", "LinkedAccount", "111111111111"),
		mkMetric("EstimatedCharges", "Currency", "USD", "LinkedAccount", "111111111111", "ServiceName", "Compute"),
		mkMetric("EstimatedCharges", "Currency", "EUR", "ServiceName", "Compute"),
		mkMetric("EstimatedCharges", "Currency", "USD", "ServiceName", "Compute", "Region", "global"),
		mkMetric("OtherMetric", "Currency", "USD", "ServiceName", "Compute"),
	}
	tests := map[string]struct {
		profile string
		want    []string
	}{
		"service": {
			profile: "billing_service",
			want:    []string{"USD", "Compute"},
		},
		"linked account": {
			profile: "billing_linked_account",
			want:    []string{"USD", "111111111111"},
		},
		"linked account and service": {
			profile: "billing_linked_account_service",
			want:    []string{"USD", "111111111111", "Compute"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile, ok := catalog.Get(tc.profile)
			require.True(t, ok)
			client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(listed, "")}}

			instances, err := discoverOneProfile(context.Background(), client, profile, false)

			require.NoError(t, err)
			require.Len(t, instances, 1)
			assert.Equal(t, tc.want, instances[0].DimensionValues)
		})
	}
}

func TestBillingProfiles_DiscoveryHandlesDifferentAccountVisibility(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	profileNames := []string{"billing_service", "billing_linked_account", "billing_linked_account_service"}
	profiles := make([]cwprofiles.ResolvedProfile, 0, len(profileNames))
	for _, name := range profileNames {
		profile, ok := catalog.Get(name)
		require.True(t, ok)
		profiles = append(profiles, resolved(name, profile))
	}
	tests := map[string]struct {
		listed []cwtypes.Metric
		want   map[string]int
	}{
		"management account exposes linked-account grains": {
			listed: []cwtypes.Metric{
				mkMetric("EstimatedCharges", "Currency", "USD", "ServiceName", "Compute"),
				mkMetric("EstimatedCharges", "Currency", "USD", "LinkedAccount", "111111111111"),
				mkMetric("EstimatedCharges", "Currency", "USD", "LinkedAccount", "111111111111", "ServiceName", "Compute"),
			},
			want: map[string]int{"billing_service": 1, "billing_linked_account": 1, "billing_linked_account_service": 1},
		},
		"standalone account can expose only service grain": {
			listed: []cwtypes.Metric{
				mkMetric("EstimatedCharges", "Currency", "USD", "ServiceName", "Compute"),
			},
			want: map[string]int{"billing_service": 1},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(tc.listed, "")}}
			instances, err := scanDiscoveryGroupForTest(context.Background(), client, discoveryGroup{
				Namespace: "AWS/Billing",
				Profiles:  profiles,
			})

			require.NoError(t, err)
			for _, profile := range profileNames {
				assert.Len(t, instances[profile], tc.want[profile])
			}
		})
	}
}

func TestBillingTotal_BuildsStaticQueryWithoutDiscovery(t *testing.T) {
	c := billingTestCollector(t, "billing_total")

	assert.Empty(t, c.discoveryGroups())
	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 1)
	query := plan[0]

	assert.Equal(t, "billing_total.estimated_charges_maximum", query.seriesName)
	assert.Equal(t, "AWS/Billing", aws.ToString(query.query.MetricStat.Metric.Namespace))
	assert.Equal(t, "EstimatedCharges", aws.ToString(query.query.MetricStat.Metric.MetricName))
	assert.Equal(t, "Maximum", aws.ToString(query.query.MetricStat.Stat))
	assert.Equal(t, int32(600), aws.ToInt32(query.query.MetricStat.Period))
	assert.Equal(t, cwquery.Policy{
		Period: 10 * time.Minute, Lookback: 24 * time.Hour, PublicationDelay: cwquery.DefaultPublicationDelay,
	}, query.policy)
	assert.Equal(t, "000000000000", labelValue(query.labels, "account_id"))
	assert.Equal(t, "us-east-1", labelValue(query.labels, "region"))
	assert.Len(t, query.labels, 2, "the constant Currency dimension is not an identity label")
	require.Len(t, query.query.MetricStat.Metric.Dimensions, 1)
	assert.Equal(t, "Currency", aws.ToString(query.query.MetricStat.Metric.Dimensions[0].Name))
	assert.Equal(t, "USD", aws.ToString(query.query.MetricStat.Metric.Dimensions[0].Value))
	assert.False(t, query.nilAsZero, "a missing gauge datapoint must remain a gap")
}

func TestBillingTotal_QueryWindowCanSpanUTCMonthBoundary(t *testing.T) {
	c := billingTestCollector(t, "billing_total")
	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 1)
	now := time.Date(2026, time.August, 1, 0, 5, 0, 0, time.UTC)

	start, end := queryWindow(now, plan[0].policy)

	assert.Equal(t, time.Date(2026, time.July, 30, 23, 50, 0, 0, time.UTC), start)
	assert.Equal(t, time.Date(2026, time.July, 31, 23, 50, 0, 0, time.UTC), end)
}

func TestBillingTotal_RefreshDiscoveryIsNoOp(t *testing.T) {
	c := billingTestCollector(t, "billing_total")
	fake := &fakeCloudWatch{}
	useFakeClient(c, fake)
	base := time.Unix(1_000, 0)
	c.now = func() time.Time { return base }
	c.discoverySig = "unchanged"
	sentinel := testStructuralID("static-query")
	c.queryPlan = []plannedQuery{{key: sentinel}}
	c.planDirty = false
	c.tagFetchPlan = []tagFetchGroup{{key: tagFetchKey{target: "unchanged"}}}

	for range 2 {
		require.NoError(t, c.refreshDiscovery(context.Background()))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c.refreshDiscovery(ctx), context.Canceled)

	assert.Empty(t, fake.inputs, "static-only selection must not call ListMetrics")
	assert.True(t, c.discovery.FetchedAt.IsZero())
	assert.True(t, c.discovery.ExpiresAt.IsZero())
	assert.Nil(t, c.discovery.Instances)
	assert.Equal(t, "unchanged", c.discoverySig)
	require.Len(t, c.queryPlan, 1)
	assert.Equal(t, sentinel, c.queryPlan[0].key)
	assert.False(t, c.planDirty)
	require.Len(t, c.tagFetchPlan, 1)
	assert.Equal(t, "unchanged", c.tagFetchPlan[0].key.target)
}

func TestBillingTotal_CreatesNoResourceTagWork(t *testing.T) {
	c := billingTestCollector(t, "billing_total")
	c.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}

	c.computeTagLabelPlans()

	assert.Empty(t, c.tagLabelPlans)
	assert.False(t, c.hasTagWork())
	assert.Empty(t, c.currentTagFetchPlan())
}

func TestBillingProfiles_MixedDiscoveryFailureKeepsStaticQueries(t *testing.T) {
	c := billingTestCollector(t, "billing_total", "billing_service")
	fake := &fakeCloudWatch{err: assert.AnError}
	useFakeClient(c, fake)
	base := time.Unix(1_000, 0)
	now := base
	c.now = func() time.Time { return now }

	queries := requireCurrentQueryPlan(t, c)
	require.Len(t, queries, 1, "the static total is executable before dynamic discovery succeeds")
	require.NoError(t, c.refreshDiscovery(context.Background()))
	require.Len(t, fake.inputs, 1)
	assert.Empty(t, fake.inputs[0].RecentlyActive)
	assert.True(t, c.discovery.FetchedAt.IsZero())
	assert.Equal(t, base.Add(time.Duration(c.Discovery.RefreshEvery)*time.Second), c.discovery.ExpiresAt)
	assert.False(t, c.planDirty, "a retry disposition must preserve the static query plan")
	require.Len(t, requireCurrentQueryPlan(t, c), 1)

	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Len(t, fake.inputs, 1, "the failed dynamic group retries after refresh_every, not every collect")

	now = base.Add(time.Duration(c.Discovery.RefreshEvery+1) * time.Second)
	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Len(t, fake.inputs, 2)
}

func TestBillingProfiles_RejectUnsupportedRegion(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Regions = []string{"us-west-2"}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"billing_total"}}
	cfg.applyDefaults()

	_, _, err = compileConfig(cfg, catalog)

	assert.ErrorContains(t, err, "none of regions [us-west-2] are supported")
}

func TestBillingProfiles_RequireInheritedResourceTagFilterToBeCleared(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	filter := []ResourceTagFilterConfig{{Key: "managed-by", Values: []string{"platform"}}}
	tests := map[string]struct {
		clear   bool
		wantErr string
	}{
		"inherited filter is rejected": {
			wantErr: "has no safe tag association",
		},
		"explicit empty filter is accepted": {
			clear: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.RuleDefaults.Filters.ResourceTags = filter
			cfg.Rules[0].Profiles = &ProfileSelectorConfig{
				Defaults: &defaults,
				Include:  []string{"billing_total", "billing_service"},
			}
			if tc.clear {
				empty := []ResourceTagFilterConfig{}
				cfg.Rules[0].Filters = &RuleFiltersConfig{ResourceTags: &empty}
			}
			cfg.applyDefaults()

			plan, _, err := compileConfig(cfg, catalog)
			if tc.wantErr != "" {
				assert.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, plan.Scopes, 2)
		})
	}
}

func TestBillingTotal_StaticInstancesRespectMaxInstances(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = append(cfg.Targets, TargetConfig{Name: "second", Credentials: "sdk_default"})
	cfg.Rules[0].Targets = []string{"base", "second"}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"billing_total"}}
	cfg.Limits.MaxInstances = 1
	cfg.applyDefaults()
	plan, _, err := compileConfig(cfg, catalog)
	require.NoError(t, err)

	c := New()
	c.Config = cfg
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{
		"base":   {target: plan.Targets[0], accountID: "000000000000"},
		"second": {target: plan.Targets[1], accountID: "111111111111"},
	}

	_, err = c.buildQueryPlan()

	assert.ErrorContains(t, err, "more than limits.max_instances=1 final instances")
}

func profileNames(profiles []cwprofiles.ResolvedProfile) []string {
	names := make([]string, len(profiles))
	for i, profile := range profiles {
		names[i] = profile.Name
	}
	return names
}
