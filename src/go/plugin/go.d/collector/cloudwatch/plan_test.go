// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

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
	assert.Nil(t, plan, "overflow must not return a partial plan plan")
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

	raised := 2 * defaultMaxDiscoveryGroups
	cfg.Limits.MaxDiscoveryGroups = raised
	plan, _, err = compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, plan.Scopes, raised)
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
