// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validBaseConfig() Config {
	return Config{
		UpdateEvery: 60,
		Credentials: []CredentialSourceConfig{
			{Name: "sdk_default", CredentialConfig: awsauth.CredentialConfig{Type: awsauth.CredentialTypeDefault}},
		},
		Targets:   []TargetConfig{{Name: "base", Credentials: "sdk_default"}},
		Rules:     []RuleConfig{{Name: "base-defaults", Targets: []string{"base"}, Regions: []string{"us-east-1"}}},
		Discovery: DiscoveryConfig{RefreshEvery: 300},
		Limits:    LimitsConfig{MaxInstances: defaultMaxInstances, MaxDiscoveryGroups: defaultMaxDiscoveryGroups},
		Timeout:   defaultTimeout,
	}
}

func TestConfig_validate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Config)
		wantErr bool
	}{
		"valid":                        {mutate: func(*Config) {}},
		"no credentials":               {mutate: func(c *Config) { c.Credentials = nil }, wantErr: true},
		"no targets":                   {mutate: func(c *Config) { c.Targets = nil }, wantErr: true},
		"no rules":                     {mutate: func(c *Config) { c.Rules = nil }, wantErr: true},
		"unknown credential reference": {mutate: func(c *Config) { c.Targets[0].Credentials = "missing" }, wantErr: true},
		"duplicate credential": {mutate: func(c *Config) {
			c.Credentials = append(c.Credentials, c.Credentials[0])
		}, wantErr: true},
		"invalid credential name": {mutate: func(c *Config) {
			c.Credentials[0].Name = "INVALID NAME"
		}, wantErr: true},
		"noncanonical credential name": {mutate: func(c *Config) {
			c.Credentials[0].Name = " sdk_default "
		}, wantErr: true},
		"empty credential name":         {mutate: func(c *Config) { c.Credentials[0].Name = "" }, wantErr: true},
		"noncanonical target name":      {mutate: func(c *Config) { c.Targets[0].Name = " base " }, wantErr: true},
		"noncanonical target reference": {mutate: func(c *Config) { c.Rules[0].Targets = []string{" base "} }, wantErr: true},
		"duplicate target":              {mutate: func(c *Config) { c.Targets = append(c.Targets, c.Targets[0]) }, wantErr: true},
		"invalid role ARN account": {mutate: func(c *Config) {
			c.Targets[0].AssumeRole = &awsauth.AssumeRoleConfig{RoleARN: "arn:aws:iam::account:role/example"}
		}, wantErr: true},
		"rule without regions":        {mutate: func(c *Config) { c.Rules[0].Regions = nil }, wantErr: true},
		"invalid region":              {mutate: func(c *Config) { c.Rules[0].Regions = []string{"global"} }, wantErr: true},
		"noncanonical region":         {mutate: func(c *Config) { c.Rules[0].Regions = []string{"us-east-1", " US-EAST-1 "} }, wantErr: true},
		"update_every below minimum":  {mutate: func(c *Config) { c.UpdateEvery = 30 }, wantErr: true},
		"refresh_every below minimum": {mutate: func(c *Config) { c.Discovery.RefreshEvery = 30 }, wantErr: true},
		"negative timeout":            {mutate: func(c *Config) { c.Timeout = confopt.Duration(-time.Second) }, wantErr: true},
		"negative instance limit":     {mutate: func(c *Config) { c.Limits.MaxInstances = -1 }, wantErr: true},
		"negative discovery group limit": {mutate: func(c *Config) {
			c.Limits.MaxDiscoveryGroups = -1
		}, wantErr: true},
		"discovery group limit above synchronous maximum": {mutate: func(c *Config) {
			c.Limits.MaxDiscoveryGroups = maxDiscoveryGroupsPerJob + 1
		}, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.mutate(&cfg)
			err := cfg.validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConfig_applyDefaults pins the zero-sentinel contract the stock config documents: a zero value
// selects the option's default rather than meaning "unlimited" or "never". An omitted key and an
// explicit 0 both decode to the same zero int, which is why the sentinel exists at all.
func TestConfig_applyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()

	assert.Equal(t, defaultUpdateEvery, cfg.UpdateEvery)
	assert.Equal(t, defaultDiscoveryRefresh, cfg.Discovery.RefreshEvery)
	assert.Equal(t, defaultMaxInstances, cfg.Limits.MaxInstances)
	assert.Equal(t, defaultMaxDiscoveryGroups, cfg.Limits.MaxDiscoveryGroups)
	assert.Equal(t, defaultTimeout, cfg.Timeout)
}

func TestConfigSchema_MatchesMetadataGroups(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestConfig_validateResourceTagConfiguration(t *testing.T) {
	filters := func(entries ...ResourceTagFilterConfig) *[]ResourceTagFilterConfig { return &entries }
	tests := map[string]struct {
		mutate  func(*Config)
		wantErr string
	}{
		"valid exact filters and labels": {mutate: func(c *Config) {
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production", "staging"}}}
			c.Rules[0].Filters = &RuleFiltersConfig{ResourceTags: filters(ResourceTagFilterConfig{Key: "team", Values: []string{"sre"}})}
			c.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "Owner", Label: "resource_owner"}}
		}},
		"filter key required": {mutate: func(c *Config) {
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Values: []string{"production"}}}
		}, wantErr: ".key must not be empty"},
		"filter values required": {mutate: func(c *Config) {
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment"}}
		}, wantErr: ".values must contain at least one value"},
		"duplicate filter key rejected": {mutate: func(c *Config) {
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{
				{Key: "environment", Values: []string{"production"}},
				{Key: "environment", Values: []string{"staging"}},
			}
		}, wantErr: "duplicate key"},
		"duplicate exact value rejected": {mutate: func(c *Config) {
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production", "production"}}}
		}, wantErr: "duplicate value"},
		"more than 50 keys rejected": {mutate: func(c *Config) {
			for i := range maxResourceTagFilters + 1 {
				c.RuleDefaults.Filters.ResourceTags = append(c.RuleDefaults.Filters.ResourceTags, ResourceTagFilterConfig{Key: fmt.Sprintf("key-%d", i), Values: []string{"value"}})
			}
		}, wantErr: "maximum is 50"},
		"more than 20 values rejected": {mutate: func(c *Config) {
			entry := ResourceTagFilterConfig{Key: "environment"}
			for i := range maxResourceTagValues + 1 {
				entry.Values = append(entry.Values, fmt.Sprintf("value-%d", i))
			}
			c.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{entry}
		}, wantErr: "maximum is 20"},
		"label key required": {mutate: func(c *Config) {
			c.Labels.ResourceTags = []ResourceTagLabelConfig{{Label: "owner"}}
		}, wantErr: ".key must not be empty"},
		"invalid emitted label rejected": {mutate: func(c *Config) {
			c.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "Owner", Label: "Bad-Label"}}
		}, wantErr: "not a valid label key"},
		"duplicate emitted label rejected": {mutate: func(c *Config) {
			c.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "Owner", Label: "owner"}, {Key: "owner", Label: "owner"}}
		}, wantErr: "duplicate label"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.mutate(&cfg)
			err := cfg.validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestConfig_validateResourceTagConfiguration_RedactsDuplicateValue(t *testing.T) {
	const sensitive = "SENSITIVE_TAG_VALUE"
	cfg := validBaseConfig()
	cfg.RuleDefaults.Filters.ResourceTags = []ResourceTagFilterConfig{{
		Key: "environment", Values: []string{sensitive, sensitive},
	}}

	err := cfg.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate value")
	assert.NotContains(t, err.Error(), sensitive)
}

func TestConfig_validateMetricSelector(t *testing.T) {
	tests := map[string]struct {
		selectors []ProfileMetricSelectorConfig
		wantErr   string
	}{
		"valid group default": {
			selectors: []ProfileMetricSelectorConfig{{
				Profile: "ec2", Statistics: []string{"Sum"},
				Include: []MetricSelectionConfig{{Name: "NetworkIn"}, {Name: "NetworkOut"}},
			}},
		},
		"valid metric override": {
			selectors: []ProfileMetricSelectorConfig{{
				Profile: "ec2", Statistics: []string{"Sum"},
				Include: []MetricSelectionConfig{
					{Name: "NetworkIn"},
					{Name: "CPUUtilization", Statistics: []string{"average"}},
				},
			}},
		},
		"empty group statistics": {
			selectors: []ProfileMetricSelectorConfig{{
				Profile: "ec2", Statistics: []string{},
				Include: []MetricSelectionConfig{{Name: "CPUUtilization", Statistics: []string{"Average"}}},
			}},
			wantErr: "statistics must contain at least one entry when present",
		},
		"empty metric statistics": {
			selectors: []ProfileMetricSelectorConfig{{
				Profile: "ec2", Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "CPUUtilization", Statistics: []string{}}},
			}},
			wantErr: "statistics must contain at least one entry when present",
		},
		"empty groups": {selectors: []ProfileMetricSelectorConfig{}, wantErr: "must contain at least one profile group"},
		"empty profile": {
			selectors: []ProfileMetricSelectorConfig{{Include: []MetricSelectionConfig{{Name: "CPUUtilization", Statistics: []string{"Average"}}}}},
			wantErr:   ".profile must not be empty",
		},
		"duplicate profile": {
			selectors: []ProfileMetricSelectorConfig{
				{Profile: "ec2", Statistics: []string{"Average"}, Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}},
				{Profile: "ec2", Statistics: []string{"Sum"}, Include: []MetricSelectionConfig{{Name: "NetworkIn"}}},
			},
			wantErr: "duplicate profile",
		},
		"empty include": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"Average"}}},
			wantErr:   "include must contain at least one metric",
		},
		"empty metric name": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"Average"}, Include: []MetricSelectionConfig{{}}}},
			wantErr:   ".name must not be empty",
		},
		"duplicate metric": {
			selectors: []ProfileMetricSelectorConfig{{
				Profile: "ec2", Statistics: []string{"Average"},
				Include: []MetricSelectionConfig{{Name: "CPUUtilization"}, {Name: "CPUUtilization", Statistics: []string{"Maximum"}}},
			}},
			wantErr: "duplicate metric",
		},
		"metric name surrounding whitespace": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"Average"}, Include: []MetricSelectionConfig{{Name: " CPUUtilization"}}}},
			wantErr:   "must not contain surrounding whitespace",
		},
		"profile statistics inherited": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}}},
		},
		"internal statistic spelling": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"sample_count"}, Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}}},
			wantErr:   "is not valid",
		},
		"duplicate group statistic after normalization": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Statistics: []string{"Average", "average"}, Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}}},
			wantErr:   "duplicate statistic",
		},
		"duplicate metric statistic after normalization": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Include: []MetricSelectionConfig{{Name: "CPUUtilization", Statistics: []string{"Average", "average"}}}}},
			wantErr:   "duplicate statistic",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			cfg.Rules[0].Metrics = tc.selectors
			err := cfg.validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

// TestConfigSchema_FormContract covers the two schema expectations that are not restatements of
// the file's own content. Nothing validates a job config against this schema — the DynCfg schema
// is a form contract, and `Config.validate` is the only enforcement layer — so describing what the
// schema currently says would pin whatever it says, including a mistake. The repo-wide rule against
// minItems on an optional array lives in the parent collector package.
func TestConfigSchema_FormContract(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	// Query timing resolves rule -> rule_defaults -> profile -> built-in. A schema default makes
	// the form emit a value the operator never chose, collapsing that chain.
	ruleQuery := schemaObjectAt(t, doc, "jsonSchema", "properties", "rules", "items", "properties", "query", "properties")
	defaultQuery := schemaObjectAt(t, doc, "jsonSchema", "properties", "rule_defaults", "properties", "query", "properties")
	for _, field := range []string{"period", "lookback", "publication_delay"} {
		for _, query := range []map[string]any{ruleQuery, defaultQuery} {
			assert.NotContainsf(t, schemaObjectAt(t, query, field), "default",
				"%s must stay omitted so runtime precedence resolves it", field)
		}
	}

	// The operator-facing bounds must track the constants that actually enforce them, or the form
	// blocks values the collector accepts.
	groups := schemaObjectAt(t, doc, "jsonSchema", "properties", "limits", "properties", "max_discovery_groups")
	assert.Equal(t, float64(defaultMaxDiscoveryGroups), groups["default"])
	assert.Equal(t, float64(maxDiscoveryGroupsPerJob), groups["maximum"])
}

func schemaObjectAt(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	var value any = root
	for _, key := range path {
		obj, ok := value.(map[string]any)
		require.Truef(t, ok, "%s parent is %T", strings.Join(path, "."), value)
		value, ok = obj[key]
		require.Truef(t, ok, "%s is missing", strings.Join(path, "."))
	}
	obj, ok := value.(map[string]any)
	require.Truef(t, ok, "%s is %T", strings.Join(path, "."), value)
	return obj
}

// TestRuleConfig_effectiveResourceTagFilters pins the nil-versus-empty contract that the pointer in
// RuleFiltersConfig exists for: a rule omitting resource_tags inherits the job default, while a rule
// setting it to an empty list disables the default for itself.
func TestRuleConfig_effectiveResourceTagFilters(t *testing.T) {
	defaults := []ResourceTagFilterConfig{{Key: "env", Values: []string{"prod"}}}
	own := []ResourceTagFilterConfig{{Key: "team", Values: []string{"sre"}}}

	tests := map[string]struct {
		filters *RuleFiltersConfig
		want    []ResourceTagFilterConfig
	}{
		"filters omitted inherits the default":       {want: defaults},
		"resource_tags omitted inherits the default": {filters: &RuleFiltersConfig{}, want: defaults},
		"empty resource_tags disables the default": {
			filters: &RuleFiltersConfig{ResourceTags: &[]ResourceTagFilterConfig{}},
			want:    []ResourceTagFilterConfig{},
		},
		"non-empty resource_tags replaces the default": {
			filters: &RuleFiltersConfig{ResourceTags: &own},
			want:    own,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rule := RuleConfig{Name: "sample", Filters: tc.filters}
			assert.Equal(t, tc.want, rule.effectiveResourceTagFilters(defaults))
		})
	}
}

func TestNormalizeRegions(t *testing.T) {
	assert.Equal(t, []string{"us-east-1", "eu-west-1"},
		normalizeRegions([]string{"us-east-1", "us-east-1", "eu-west-1", "", "eu-west-1"}))
}
