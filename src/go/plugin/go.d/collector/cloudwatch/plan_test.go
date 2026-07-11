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

func compileTestConfig(t *testing.T, cfg Config) (*collectorRuntime, error) {
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
	runtime, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Len(t, runtime.Scopes, 2)
	assert.Equal(t, []string{"ec2", "rds"}, []string{runtime.Scopes[0].Profile.Name, runtime.Scopes[1].Profile.Name})
}

func TestCompileConfig_DefaultUnsupportedRegionIsSkipped(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Rules[0].Regions = []string{"eu-west-1"}
	runtime, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, runtime.Diagnostics)
	for _, scope := range runtime.Scopes {
		assert.NotEqual(t, "cloudfront", scope.Profile.Name)
	}
}

func TestCompileConfig_DefaultUnsupportedRegionDiagnosticIsEmittedOnce(t *testing.T) {
	cfg := twoTargetConfig()
	cfg.Rules[0].Profiles = nil
	cfg.Rules[0].Regions = []string{"eu-west-1"}
	runtime, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	var matches int
	for _, diagnostic := range runtime.Diagnostics {
		if strings.Contains(diagnostic, `skips default profile "cloudfront"`) {
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
	_, err := compileTestConfig(t, cfg)
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
	runtime, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, runtime.Scopes, 1)
	assert.Equal(t, "first", runtime.Scopes[0].RuleName)
	assert.Contains(t, runtime.Diagnostics[0], "shadowed")
}

func TestCompileConfig_TargetMayUseStaticCredentialsForAssumeRole(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Credentials = map[string]awsauth.CredentialConfig{
		"bootstrap": {Type: awsauth.CredentialTypeStatic, AccessKeyID: "key", SecretAccessKey: "secret"},
	}
	cfg.Targets = []TargetConfig{{
		Name: "production", Credentials: "bootstrap",
		AssumeRole: &awsauth.AssumeRoleConfig{RoleARN: "arn:aws:iam::000000000000:role/netdata"},
	}}
	cfg.Rules[0].Targets = []string{"production"}
	runtime, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	assert.Equal(t, "production", runtime.Targets[0].Identity.Ref)
}

func TestCompileConfig_RejectsMixedPartitionsPerTarget(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Rules = []RuleConfig{{
		Name: "mixed", Targets: []string{"base"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}},
		Regions:  []string{"us-east-1", "cn-north-1"},
	}}
	_, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "spans multiple AWS partitions")
}

func TestCompileConfig_RejectsUnusedDefinitions(t *testing.T) {
	cfg := validBaseConfig()
	cfg.Credentials["unused"] = awsauth.CredentialConfig{Type: awsauth.CredentialTypeDefault}
	_, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "credential \"unused\" is not referenced")
}

func TestCompileConfig_RejectsRawLimitsBeforeCompilation(t *testing.T) {
	cfg := validBaseConfig()
	for i := 1; i <= maxTargets; i++ {
		cfg.Targets = append(cfg.Targets, TargetConfig{Name: fmt.Sprintf("target-%d", i), Credentials: "sdk_default"})
	}
	_, err := compileTestConfig(t, cfg)
	assert.ErrorContains(t, err, "'targets' contains")
	assert.ErrorContains(t, err, "maximum is 256")
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
	cfg.Rules = []RuleConfig{{
		Name: "expanded", Targets: refs, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{
			"api_gateway", "auto_scaling", "bedrock", "dynamodb", "ebs", "ec2", "efs", "eks", "elasticache",
			"elb", "eventbridge", "firehose", "kinesis", "lambda", "nat_gateway", "opensearch", "rds",
		}},
	}}

	runtime, err := compileTestConfig(t, cfg)
	assert.Nil(t, runtime, "overflow must not return a partial runtime plan")
	assert.ErrorContains(t, err, "compiled collection scopes exceed maximum")
}
