// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileTestConfig(t *testing.T, cfg Config) (*collectionPlan, []string, error) {
	t.Helper()
	catalog, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)
	cfg.applyDefaults()
	return compileConfig(cfg, catalog)
}

func TestCompileConfig_RuleSelectors(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{{
		Name: "services", Targets: []string{"base"}, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2", "rds"}},
	}}
	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, 2)
	assert.Equal(t, []string{"ec2", "rds"}, []string{plan.Scopes[0].Profile.Name, plan.Scopes[1].Profile.Name})
}

func TestCompileConfig_ExactMetricSelection(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	cfg.Rules[0].Metrics = []ProfileMetricSelectorConfig{{
		Profile: "ec2", Statistics: []string{"sum"},
		Include: []MetricSelectionConfig{
			{Name: "NetworkIn"},
			{Name: "CPUUtilization", Statistics: []string{"AVERAGE"}},
		},
	}}

	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 1)
	assert.Equal(t, []string{"ec2.cpu_utilization_average", "ec2.network_in_sum"}, compiledSeriesNames(plan.Scopes[0].SelectedSeries))
}

func TestCompileConfig_SanitizedExistingProfileParity(t *testing.T) {
	defaults := false
	profile := func(name string) *ProfileSelectorConfig {
		return &ProfileSelectorConfig{Defaults: &defaults, Include: []string{name}}
	}
	metrics := func(profile, statistic string, names ...string) []ProfileMetricSelectorConfig {
		include := make([]MetricSelectionConfig, len(names))
		for i, name := range names {
			include[i] = MetricSelectionConfig{Name: name}
		}
		return []ProfileMetricSelectorConfig{{Profile: profile, Statistics: []string{statistic}, Include: include}}
	}
	query := func(period, lookback, delay time.Duration) *cwquery.Config {
		return &cwquery.Config{
			Period:           longDuration(period),
			Lookback:         longDuration(lookback),
			PublicationDelay: longDuration(delay),
		}
	}

	rdsMetrics := metrics("rds", "Average",
		"CPUUtilization",
		"DatabaseConnections",
		"DiskQueueDepth",
		"FreeableMemory",
		"FreeStorageSpace",
		"NetworkReceiveThroughput",
		"NetworkTransmitThroughput",
		"ReadIOPS",
		"ReplicaLag",
		"SwapUsage",
		"WriteIOPS",
		"MaximumUsedTransactionIDs",
	)
	for i := range rdsMetrics[0].Include {
		if rdsMetrics[0].Include[i].Name == "ReplicaLag" {
			rdsMetrics[0].Include[i].Statistics = []string{"Maximum"}
		}
	}
	nlbFilter := []ResourceTagFilterConfig{{Key: "created_by", Values: []string{"automation"}}}

	cfg := validBaseConfig()
	cfg.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "owner", Values: []string{"platform"}}}
	cfg.Rules = []RuleConfig{
		{
			Name: "rds", Targets: []string{"base"}, Profiles: profile("rds"), Metrics: rdsMetrics,
			Regions: []string{"us-east-1"}, Query: query(time.Minute, time.Minute, 5*time.Minute),
		},
		{
			Name: "vpn", Targets: []string{"base"}, Profiles: profile("vpn"), Metrics: metrics("vpn", "Average", "TunnelState"),
			Regions: []string{"us-east-1"}, Query: query(time.Minute, time.Minute, 5*time.Minute),
		},
		{
			Name: "nlb", Targets: []string{"base"}, Profiles: profile("nlb"), Metrics: metrics("nlb", "Sum", "ProcessedBytes"),
			Regions: []string{"us-east-1"}, Filters: &RuleFiltersConfig{ResourceTags: &nlbFilter},
			Query: query(6*time.Hour, 6*time.Hour, 5*time.Minute),
		},
		{
			Name: "lambda-sparse", Targets: []string{"base"}, Profiles: profile("lambda"),
			Metrics: metrics("lambda", "Sum", "Invocations", "Errors"), Regions: []string{"us-east-1"},
			Query: query(2*time.Hour, 6*time.Hour, cwquery.DefaultPublicationDelay),
		},
		{
			Name: "lambda-daily", Targets: []string{"base"}, Profiles: profile("lambda"),
			Metrics: metrics("lambda", "Sum", "Invocations", "Errors"), Regions: []string{"us-east-1"},
			Query: query(time.Hour, 24*time.Hour, cwquery.DefaultPublicationDelay),
		},
		{
			Name: "sqs", Targets: []string{"base"}, Profiles: profile("sqs"),
			Metrics: []ProfileMetricSelectorConfig{{
				Profile: "sqs", Statistics: []string{"Sum"},
				Include: []MetricSelectionConfig{
					{Name: "NumberOfMessagesSent"},
					{Name: "NumberOfMessagesReceived"},
					{Name: "NumberOfMessagesDeleted"},
					{Name: "ApproximateAgeOfOldestMessage", Statistics: []string{"Maximum"}},
				},
			}},
			Regions: []string{"us-east-1"}, Query: query(5*time.Minute, 5*time.Minute, cwquery.DefaultPublicationDelay),
		},
	}

	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 5)

	wantScopes := []struct {
		profile string
		series  []string
		policy  cwquery.Policy
		rates   map[string]bool
	}{
		{
			profile: "rds",
			series: []string{
				"rds.cpu_utilization_average",
				"rds.database_connections_average",
				"rds.freeable_memory_average",
				"rds.free_storage_space_average",
				"rds.read_iops_average",
				"rds.write_iops_average",
				"rds.replica_lag_maximum",
				"rds.maximum_used_transaction_ids_average",
				"rds.disk_queue_depth_average",
				"rds.network_receive_throughput_average",
				"rds.network_transmit_throughput_average",
				"rds.swap_usage_average",
			},
			policy: cwquery.Policy{Period: time.Minute, Lookback: time.Minute, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "vpn",
			series:  []string{"vpn.tunnel_state_average"},
			policy:  cwquery.Policy{Period: time.Minute, Lookback: time.Minute, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "nlb",
			series:  []string{"nlb.processed_bytes_sum"},
			policy:  cwquery.Policy{Period: 6 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 5 * time.Minute},
			rates:   map[string]bool{"nlb.processed_bytes_sum": true},
		},
		{
			profile: "lambda",
			series:  []string{"lambda.invocations_sum", "lambda.errors_sum"},
			policy:  cwquery.Policy{Period: 2 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: cwquery.DefaultPublicationDelay},
			rates:   map[string]bool{"lambda.invocations_sum": true, "lambda.errors_sum": true},
		},
		{
			profile: "sqs",
			series: []string{
				"sqs.number_of_messages_sent_sum",
				"sqs.number_of_messages_received_sum",
				"sqs.number_of_messages_deleted_sum",
				"sqs.approximate_age_of_oldest_message_maximum",
			},
			policy: cwquery.Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: cwquery.DefaultPublicationDelay},
			rates: map[string]bool{
				"sqs.number_of_messages_sent_sum":     true,
				"sqs.number_of_messages_received_sum": true,
				"sqs.number_of_messages_deleted_sum":  true,
			},
		},
	}
	for i, want := range wantScopes {
		scope := plan.Scopes[i]
		assert.Equal(t, want.profile, scope.Profile.Name)
		assert.Equal(t, want.series, compiledSeriesNames(scope.SelectedSeries))
		for _, series := range scope.SelectedSeries {
			assert.Equal(t, want.policy, series.Policy, series.Name)
			metric := scope.Profile.Config.Metrics[series.MetricIndex]
			assert.Equal(t, want.rates[series.Name], metric.Rate, series.Name)
		}
	}

	require.Len(t, diagnostics, 1)
	assert.Contains(t, diagnostics[0], `rule "lambda-daily" has 2 metric selection(s) shadowed`)
	assert.Contains(t, diagnostics[0], `rule "lambda-sparse" owns`)
}

func TestCompileConfig_MetricSelectionExpandsMultipleStatistics(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"lambda"}}
	cfg.Rules[0].Metrics = []ProfileMetricSelectorConfig{{
		Profile: "lambda",
		Include: []MetricSelectionConfig{{
			Name:       "Duration",
			Statistics: []string{"Average", "Maximum", "p90"},
		}},
	}}

	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 1)
	assert.Equal(t, []string{
		"lambda.duration_average",
		"lambda.duration_maximum",
		"lambda.duration_p90",
	}, compiledSeriesNames(plan.Scopes[0].SelectedSeries))
}

func TestCompileConfig_MetricSelectionRejectsUnknownOrUnselectedEntries(t *testing.T) {
	defaults := false
	tests := map[string]struct {
		group   ProfileMetricSelectorConfig
		message string
		path    string
	}{
		"profile not selected by rule": {
			group: ProfileMetricSelectorConfig{
				Profile: "rds", Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "CPUUtilization"}},
			},
			message: "not selected by this rule",
		},
		"unknown MetricName": {
			group: ProfileMetricSelectorConfig{
				Profile: "ec2", Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "DoesNotExist"}},
			},
			message: "unknown MetricName",
		},
		"statistic not exported": {
			group: ProfileMetricSelectorConfig{
				Profile: "ec2", Statistics: []string{"Sum"},
				Include: []MetricSelectionConfig{{Name: "CPUUtilization"}},
			},
			message: "is not exported",
			path:    "rules[0].metrics[0].statistics[0]",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
			cfg.Rules[0].Metrics = []ProfileMetricSelectorConfig{tc.group}
			_, _, err := compileTestConfig(t, cfg)
			assert.ErrorContains(t, err, tc.message)
			if tc.path != "" {
				assert.ErrorContains(t, err, tc.path)
			}
		})
	}
}

func TestCompileConfig_PartialMetricShadowingSharesTagMembership(t *testing.T) {
	defaults := false
	profileSelector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	metrics := func(entries ...MetricSelectionConfig) []ProfileMetricSelectorConfig {
		return []ProfileMetricSelectorConfig{{Profile: "ec2", Include: entries}}
	}
	cpu := MetricSelectionConfig{Name: "CPUUtilization", Statistics: []string{"Average"}}
	network := MetricSelectionConfig{Name: "NetworkIn", Statistics: []string{"Sum"}}
	cfg := validBaseConfig()
	cfg.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg.Rules = []RuleConfig{
		{Name: "first", Targets: []string{"base"}, Profiles: profileSelector, Metrics: metrics(cpu), Regions: []string{"us-east-1"}},
		{Name: "second", Targets: []string{"base"}, Profiles: profileSelector, Metrics: metrics(network, cpu), Regions: []string{"us-east-1"}},
	}

	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 2)
	assert.Equal(t, []string{"ec2.cpu_utilization_average"}, compiledSeriesNames(plan.Scopes[0].SelectedSeries))
	assert.Equal(t, []string{"ec2.network_in_sum"}, compiledSeriesNames(plan.Scopes[1].SelectedSeries))
	assert.Equal(t, plan.Scopes[0].TagMembershipID, plan.Scopes[1].TagMembershipID)
	assert.Contains(t, strings.Join(diagnostics, "\n"), "1 metric selection(s) shadowed")
}

func compiledSeriesNames(series []compiledSeries) []string {
	names := make([]string, len(series))
	for i, item := range series {
		names[i] = item.Name
	}
	return names
}

func TestCompileConfig_RuleSelectorDefaultsAndExclusions(t *testing.T) {
	t.Run("omitted selector uses enabled defaults", func(t *testing.T) {
		plan, _, err := compileTestConfig(t, validBaseConfig())
		require.NoError(t, err)
		require.NotEmpty(t, plan.Scopes)
		for _, scope := range plan.Scopes {
			assert.False(t, scope.Profile.Config.Disabled)
		}
	})

	t.Run("defaults plus explicit disabled profile", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Rules[0].Profiles = &ProfileSelectorConfig{Include: []string{"alb_target"}}
		plan, _, err := compileTestConfig(t, cfg)
		require.NoError(t, err)
		assert.Contains(t, scopeProfileNames(plan.Scopes), "alb_target")
	})

	t.Run("exclude-only removes a default profile", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.Rules[0].Profiles = &ProfileSelectorConfig{Exclude: []string{"ec2"}}
		plan, _, err := compileTestConfig(t, cfg)
		require.NoError(t, err)
		assert.NotContains(t, scopeProfileNames(plan.Scopes), "ec2")
	})
}

func TestCompileConfig_RejectsInvalidRuleReferences(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Config)
		message string
	}{
		"include exclude overlap": {
			mutate: func(cfg *Config) {
				cfg.Rules[0].Profiles = &ProfileSelectorConfig{Include: []string{"ec2"}, Exclude: []string{"ec2"}}
			},
			message: "includes and excludes",
		},
		"duplicate profile": {
			mutate:  func(cfg *Config) { cfg.Rules[0].Profiles = &ProfileSelectorConfig{Include: []string{"ec2", "ec2"}} },
			message: "duplicate profile",
		},
		"unknown profile": {
			mutate:  func(cfg *Config) { cfg.Rules[0].Profiles = &ProfileSelectorConfig{Include: []string{"missing"}} },
			message: "unknown profile",
		},
		"duplicate target reference": {
			mutate:  func(cfg *Config) { cfg.Rules[0].Targets = []string{"base", "base"} },
			message: "duplicate target",
		},
		"unknown target reference": {
			mutate:  func(cfg *Config) { cfg.Rules[0].Targets = []string{"missing"} },
			message: "unknown target",
		},
		"unused target": {
			mutate: func(cfg *Config) {
				cfg.Targets = append(cfg.Targets, TargetConfig{Name: "unused", Credentials: "sdk_default"})
			},
			message: "not referenced by any rule",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.mutate(&cfg)
			_, _, err := compileTestConfig(t, cfg)
			assert.ErrorContains(t, err, tc.message)
		})
	}
}

func TestCompileConfig_RejectsRoleRegionPartitionMismatch(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Targets[0].AssumeRole = &awsauth.AssumeRoleConfig{RoleARN: "arn:aws-cn:iam::000000000000:role/netdata"}
	_, _, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "does not match selected region partition")
}

func scopeProfileNames(scopes []collectionScope) []string {
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, scope.Profile.Name)
	}
	return names
}

func TestCompileConfig_DefaultUnsupportedRegionIsSkipped(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Rules[0].Regions = []string{"eu-west-1"}
	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, diagnostics)
	for _, scope := range plan.Scopes {
		assert.NotEqual(t, "cloudfront", scope.Profile.Name)
	}
}

func TestCompileConfig_DefaultUnsupportedRegionDiagnosticIsEmittedOnce(t *testing.T) {
	cfg := twoTargetConfig()
	cfg.Rules[0].Profiles = nil
	cfg.Rules[0].Regions = []string{"eu-west-1"}
	_, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	var matches int
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, "cloudfront") {
			matches++
		}
	}
	assert.Equal(t, 1, matches)
}

func TestCompileConfig_ExplicitUnsupportedRegionFails(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Rules[0].Regions = []string{"eu-west-1"}
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"cloudfront"}}
	_, _, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "explicitly includes profile \"cloudfront\"")
}

func TestCompileConfig_StaticScopeFirstRuleWins(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	selector := &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}}
	cfg.Rules = []RuleConfig{
		{Name: "first", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}},
		{Name: "second", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}},
	}
	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 1)
	assert.Contains(t, diagnostics[0], "shadowed")
	assert.Contains(t, diagnostics[0], `rule "first" owns`)
}

func TestCompileConfig_ResourceTagFilterInheritanceReplacementAndDisable(t *testing.T) {
	defaults := false
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	override := []ResourceTagFilterConfig{{Key: "team", Values: []string{"sre"}}}
	disabled := []ResourceTagFilterConfig{}
	cfg := validBaseConfig()
	cfg.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg.Rules = []RuleConfig{
		{Name: "inherited", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}},
		{Name: "replaced", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}, Filters: &RuleFiltersConfig{ResourceTags: &override}},
		{Name: "disabled", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}, Filters: &RuleFiltersConfig{ResourceTags: &disabled}},
	}

	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Empty(t, diagnostics)
	require.Len(t, plan.Scopes, 3, "distinct effective predicates remain ordered policy scopes")
	assert.Equal(t, []resourceTagFilter{{key: "environment", values: []string{"production"}}}, plan.Scopes[0].TagFilter)
	assert.Equal(t, []resourceTagFilter{{key: "team", values: []string{"sre"}}}, plan.Scopes[1].TagFilter)
	assert.Empty(t, plan.Scopes[2].TagFilter)
}

func TestCompileConfig_EquivalentResourceTagFiltersDeduplicate(t *testing.T) {
	defaults := false
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	first := []ResourceTagFilterConfig{
		{Key: "team", Values: []string{"platform", "sre"}},
		{Key: "environment", Values: []string{"production"}},
	}
	second := []ResourceTagFilterConfig{
		{Key: "environment", Values: []string{"production"}},
		{Key: "team", Values: []string{"sre", "platform"}},
	}
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{
		{Name: "owner", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}, Filters: &RuleFiltersConfig{ResourceTags: &first}},
		{Name: "duplicate", Targets: []string{"base"}, Profiles: selector, Regions: []string{"us-east-1"}, Filters: &RuleFiltersConfig{ResourceTags: &second}},
	}

	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, 1)
	require.Len(t, diagnostics, 1)
	assert.Contains(t, diagnostics[0], "shadowed")
}

func TestCompileConfig_ResourceTagFilterUnsupportedProfiles(t *testing.T) {
	filter := []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}

	t.Run("default selection skips unsupported profiles", func(t *testing.T) {
		cfg := validBaseConfig()
		cfg.RuleDefaults.Filters.ResourceTags = filter
		plan, diagnostics, err := compileTestConfig(t, cfg)
		require.NoError(t, err)
		assert.NotContains(t, scopeProfileNames(plan.Scopes), "api_gateway")
		assert.Contains(t, strings.Join(diagnostics, "\n"), "api_gateway")
	})

	t.Run("explicit unsupported profile fails", func(t *testing.T) {
		defaults := false
		cfg := validBaseConfig()
		cfg.RuleDefaults.Filters.ResourceTags = filter
		cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"api_gateway"}}
		_, _, err := compileTestConfig(t, cfg)
		assert.ErrorContains(t, err, "no safe tag association")
	})

	t.Run("explicit empty override permits unsupported profile unfiltered", func(t *testing.T) {
		defaults := false
		disabled := []ResourceTagFilterConfig{}
		cfg := validBaseConfig()
		cfg.RuleDefaults.Filters.ResourceTags = filter
		cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"api_gateway"}}
		cfg.Rules[0].Filters = &RuleFiltersConfig{ResourceTags: &disabled}
		plan, _, err := compileTestConfig(t, cfg)
		require.NoError(t, err)
		require.Len(t, plan.Scopes, 1)
		assert.Empty(t, plan.Scopes[0].TagFilter)
	})
}

func TestCompileConfig_TargetMayUseStaticCredentialsForAssumeRole(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Credentials = []CredentialSourceConfig{
		{
			Name: "bootstrap",
			CredentialConfig: awsauth.CredentialConfig{
				Type: awsauth.CredentialTypeStatic,
				TypeStatic: &awsauth.StaticCredentialConfig{
					AccessKeyID: "key", SecretAccessKey: "secret",
				},
			},
		},
	}
	cfg.Targets = []TargetConfig{{
		Name: "production", Credentials: "bootstrap",
		AssumeRole: &awsauth.AssumeRoleConfig{RoleARN: "arn:aws:iam::000000000000:role/netdata"},
	}}
	cfg.Rules[0].Targets = []string{"production"}
	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Equal(t, "production", plan.Targets[0].Identity.Ref)
}

func TestCompileConfig_RejectsMixedPartitionsPerTarget(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{{
		Name: "mixed", Targets: []string{"base"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}},
		Regions:  []string{"us-east-1", "cn-north-1"},
	}}
	_, _, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "spans multiple AWS partitions")
}

func TestCompileConfig_RejectsUnusedDefinitions(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Credentials = append(cfg.Credentials, CredentialSourceConfig{
		Name: "unused", CredentialConfig: awsauth.CredentialConfig{Type: awsauth.CredentialTypeDefault},
	})
	_, _, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "credential \"unused\" is not referenced")
}

func TestCompileConfig_RejectsRawLimitsBeforeCompilation(t *testing.T) {
	cfg := validBaseConfig()
	for i := 1; i <= 64; i++ {
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: fmt.Sprintf("target-%d", i), Credentials: "sdk_default"})
	}
	_, _, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "'targets' contains")
	assert.ErrorContains(t, err, "maximum is 64")
	assert.NotContains(t, err.Error(), "must match", "raw cap overflow must fail before walking oversized target entries")
}

func TestCompileConfig_RejectsMetricSelectorOverflowBeforeEntryValidation(t *testing.T) {
	expandedMetrics := make([]MetricSelectionConfig, 129)
	for i := range expandedMetrics {
		expandedMetrics[i].Statistics = []string{"Average", "Maximum"}
	}
	tests := map[string]struct {
		metrics []ProfileMetricSelectorConfig
		message string
	}{
		"profile groups": {
			metrics: make([]ProfileMetricSelectorConfig, maxReferencesPerRule+1),
			message: "rules[0].metrics contains 257 entries; maximum is 256",
		},
		"metrics in group": {
			metrics: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"Average"}, Include: make([]MetricSelectionConfig, maxReferencesPerRule+1)}},
			message: "rules[0].metrics[0].include contains 257 entries; maximum is 256",
		},
		"statistics in group": {
			metrics: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: make([]string, maxReferencesPerRule+1), Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}}},
			message: "rules[0].metrics[0].statistics contains 257 entries; maximum is 256",
		},
		"expanded selections": {
			metrics: []ProfileMetricSelectorConfig{{Profile: "ec2", Include: expandedMetrics}},
			message: "rules[0].metrics expands to more than 256 metric/statistic selections",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Rules[0].Metrics = tc.metrics
			_, _, err := compileTestConfig(t, cfg)
			assert.ErrorContains(t, err, tc.message)
			assert.NotContains(t, err.Error(), "profile must not be empty", "raw bounds must fail before walking oversized entries")
		})
	}
}

func TestCompileConfig_AggregatesShadowDiagnosticsPerRule(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = nil
	var refs []string
	for i := range 16 {
		name := fmt.Sprintf("target-%d", i)
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: name, Credentials: "sdk_default"})
		refs = append(refs, name)
	}
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}}
	cfg.Rules = []RuleConfig{
		{Name: "owner", Targets: refs, Profiles: selector, Regions: []string{"us-east-1"}},
		{Name: "shadowed", Targets: refs, Profiles: selector, Regions: []string{"us-east-1"}},
	}

	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, 16)
	assert.Len(t, diagnostics, 1, "one later rule produces one bounded aggregate diagnostic")
	assert.Contains(t, diagnostics[0], fmt.Sprintf("%d metric selection(s)", 16*len(plan.Scopes[0].SelectedSeries)))
}

func TestCompileConfig_RejectsCandidateScopeAmplification(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = nil
	var refs []string
	for i := range maxTargets {
		name := fmt.Sprintf("target-%d", i)
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: name, Credentials: "sdk_default"})
		refs = append(refs, name)
	}
	selector := &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"s3", "s3_requests"}}
	cfg.Rules = nil
	for i := range 129 {
		cfg.Rules = append(cfg.Rules, RuleConfig{
			Name: fmt.Sprintf("rule-%d", i), Targets: refs, Profiles: selector, Regions: []string{"us-east-1"},
		})
	}

	plan, _, err := compileTestConfig(t, cfg)
	assert.Nil(t, plan)
	assert.ErrorContains(t, err, "candidate collection scopes exceed maximum")
}

func TestCompileConfig_RejectsCompiledScopeOverflowWithoutPartialPlan(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Targets = nil
	refs := make([]string, 0, maxTargets)
	for i := range maxTargets {
		name := fmt.Sprintf("target-%d", i)
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: name, Credentials: "sdk_default"})
		refs = append(refs, name)
	}
	cfg.Rules = nil
	for i := range 65 {
		filters := []ResourceTagFilterConfig{{Key: "rule", Values: []string{fmt.Sprintf("%d", i)}}}
		cfg.Rules = append(cfg.Rules, RuleConfig{
			Name: fmt.Sprintf("expanded-%d", i), Targets: refs, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}},
			Filters:  &RuleFiltersConfig{ResourceTags: &filters},
		})
	}

	plan, _, err := compileTestConfig(t, cfg)
	assert.Nil(t, plan, "overflow must not return a partial plan")
	assert.ErrorContains(t, err, "compiled collection scopes exceed maximum")
}

func TestCompileConfig_DiscoveryGroupLimit(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Targets = nil
	var refs []string
	for i := range maxTargets {
		name := fmt.Sprintf("target-%d", i)
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: name, Credentials: "sdk_default"})
		refs = append(refs, name)
	}
	cfg.Rules = []RuleConfig{{
		Name: "exact-limit", Targets: refs, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
	}}

	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, defaultMaxDiscoveryGroups)

	cfg.Rules[0].Regions = []string{"us-east-1", "us-west-2"}
	plan, _, err = compileTestConfig(t, cfg)
	assert.Nil(t, plan)
	assert.ErrorContains(t, err, "derives 65 discovery groups")
	assert.ErrorContains(t, err, "limits.max_discovery_groups=64")
	assert.ErrorContains(t, err, "raise the safeguard")
	assert.ErrorContains(t, err, "split the collection across multiple jobs")

	raised := maxDiscoveryGroupsPerJob
	cfg.Limits.MaxDiscoveryGroups = raised
	cfg.Rules[0].Targets = refs[:raised/len(cfg.Rules[0].Regions)]
	cfg.Targets = cfg.Targets[:len(cfg.Rules[0].Targets)]
	plan, _, err = compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, raised)

	cfg.Rules[0].Regions = append(cfg.Rules[0].Regions, "eu-west-1")
	plan, _, err = compileTestConfig(t, cfg)
	assert.Nil(t, plan)
	require.ErrorContains(t, err, "derives 101 discovery groups")
	assert.NotContains(t, err.Error(), "raise the safeguard")
	assert.Contains(t, err.Error(), "split the collection across multiple jobs")
}

func TestCompileConfig_DiscoveryGroupsShareTargetRegionNamespace(t *testing.T) {
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"s3", "s3_requests"}}

	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	compiler := newPlanCompiler(cfg, catalog)
	plan, _, err := compiler.compile()
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, 2)
	assert.Len(t, compiler.discoveryGroups, 1, "profiles sharing target, region, and namespace share one discovery group")
}
