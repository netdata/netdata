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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
		"duplicate target": {mutate: func(c *Config) { c.Targets = append(c.Targets, c.Targets[0]) }, wantErr: true},
		"invalid role ARN account": {mutate: func(c *Config) {
			c.Targets[0].AssumeRole = &awsauth.AssumeRoleConfig{RoleARN: "arn:aws:iam::account:role/example"}
		}, wantErr: true},
		"rule without regions":        {mutate: func(c *Config) { c.Rules[0].Regions = nil }, wantErr: true},
		"invalid region":              {mutate: func(c *Config) { c.Rules[0].Regions = []string{"global"} }, wantErr: true},
		"noncanonical region":         {mutate: func(c *Config) { c.Rules[0].Regions = []string{"us-east-1", " US-EAST-1 "} }, wantErr: true},
		"update_every below minimum":  {mutate: func(c *Config) { c.UpdateEvery = 30 }, wantErr: true},
		"refresh_every below minimum": {mutate: func(c *Config) { c.Discovery.RefreshEvery = 30 }, wantErr: true},
		"negative timeout":            {mutate: func(c *Config) { c.Timeout = confopt.Duration(-time.Second) }, wantErr: true},
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
		"missing effective statistics": {
			selectors: []ProfileMetricSelectorConfig{{Profile: "ec2", Include: []MetricSelectionConfig{{Name: "CPUUtilization"}}}},
			wantErr:   "must define statistics or inherit them",
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

func TestConfigSchema_RuntimeContract(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc struct {
		JSONSchema struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.ElementsMatch(t, []string{"credentials", "targets", "rules"}, doc.JSONSchema.Required)
	for _, key := range []string{
		"update_every", "autodetection_retry", "vnode", "credentials", "targets", "rules",
		"rule_defaults", "labels", "limits", "discovery", "timeout",
	} {
		assert.Contains(t, doc.JSONSchema.Properties, key)
	}
	assert.NotContains(t, doc.JSONSchema.Properties, "defaults")
	assert.NotContains(t, doc.JSONSchema.Properties, "tags")
	var fullDoc map[string]any
	require.NoError(t, json.Unmarshal(data, &fullDoc))
	discoveryGroupLimit := schemaObjectAt(t, fullDoc, "jsonSchema", "properties", "limits", "properties", "max_discovery_groups")
	assert.Equal(t, float64(defaultMaxDiscoveryGroups), discoveryGroupLimit["default"])
	assert.Equal(t, float64(maxDiscoveryGroupsPerJob), discoveryGroupLimit["maximum"])
	for key, want := range map[string]string{
		"credentials": `[{"name":"sdk_default","type":"default"}]`,
		"targets":     `[{"name":"base","credentials":"sdk_default"}]`,
		"rules":       `[{"name":"base-defaults","targets":["base"],"regions":["us-east-1"]}]`,
	} {
		var property struct {
			Default json.RawMessage `json:"default"`
		}
		require.NoError(t, json.Unmarshal(doc.JSONSchema.Properties[key], &property))
		assert.JSONEq(t, want, string(property.Default), key)
	}
	assert.Equal(t, 2, strings.Count(string(data), `"sensitive": true`), "only secret access key and session token are sensitive widgets")
	assert.NotContains(t, string(data), `"additionalProperties"`)
}

func TestConfigSchema_DynCfgUX(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	properties := schemaObjectAt(t, doc, "jsonSchema", "properties")
	uiSchema := schemaObjectAt(t, doc, "uiSchema")
	assert.Equal(t, "tabs", uiSchema["ui:flavour"])

	wantTabs := []struct {
		title  string
		fields []string
	}{
		{title: "Base", fields: []string{"update_every", "autodetection_retry", "timeout", "limits"}},
		{title: "Credentials", fields: []string{"credentials"}},
		{title: "Targets", fields: []string{"targets"}},
		{title: "Collection Rules", fields: []string{"rule_defaults", "rules"}},
		{title: "Resource Tags", fields: []string{"labels"}},
		{title: "Discovery", fields: []string{"discovery"}},
		{title: "Virtual Node", fields: []string{"vnode"}},
	}
	tabs, ok := schemaObjectAt(t, doc, "uiSchema", "ui:options")["tabs"].([]any)
	require.Truef(t, ok, "uiSchema.ui:options.tabs is %T", schemaObjectAt(t, doc, "uiSchema", "ui:options")["tabs"])
	require.Len(t, tabs, len(wantTabs))

	seen := make(map[string]int, len(properties))
	for i, raw := range tabs {
		tab, ok := raw.(map[string]any)
		require.Truef(t, ok, "tab %d is %T", i, raw)
		assert.Equal(t, wantTabs[i].title, tab["title"])
		fields := schemaStringSlice(t, tab["fields"], "tab fields")
		assert.Equal(t, wantTabs[i].fields, fields)
		for _, field := range fields {
			assert.Containsf(t, properties, field, "tab %q references unknown field %q", wantTabs[i].title, field)
			seen[field]++
		}
	}
	for field := range properties {
		assert.Equalf(t, 1, seen[field], "top-level schema field %q tab references", field)
	}

	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "credentials")["ui:listFlavour"])
	assert.Equal(t, "sdk_default", schemaObjectAt(t, doc, "uiSchema", "credentials", "items", "name")["ui:placeholder"])
	assert.Equal(t, "radio", schemaObjectAt(t, doc, "uiSchema", "credentials", "items", "type")["ui:widget"])
	assert.Equal(t, true, schemaObjectAt(t, doc, "uiSchema", "credentials", "items", "type", "ui:options")["inline"])
	assert.NotEmpty(t, schemaObjectAt(t, doc, "uiSchema", "credentials", "items", "type_static", "secret_access_key")["ui:help"])
	assert.Equal(t, "password", schemaObjectAt(t, doc, "uiSchema", "credentials", "items", "type_static", "secret_access_key")["ui:widget"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "targets")["ui:listFlavour"])
	assert.Equal(t, "production", schemaObjectAt(t, doc, "uiSchema", "targets", "items", "name")["ui:placeholder"])
	assert.Equal(t, "arn:aws:iam::[ACCOUNT]:role/netdata-cloudwatch",
		schemaObjectAt(t, doc, "uiSchema", "targets", "items", "assume_role", "role_arn")["ui:placeholder"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "rules")["ui:listFlavour"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "targets")["ui:listFlavour"])
	assert.Equal(t, "ec2", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "profiles", "include", "items")["ui:placeholder"])
	profileIncludeHelp, ok := schemaObjectAt(t, doc, "uiSchema", "rules", "items", "profiles", "include")["ui:help"].(string)
	require.True(t, ok)
	for _, profile := range []string{
		"privatelink_endpoint_subnet",
		"privatelink_service_az",
		"privatelink_service_load_balancer",
		"privatelink_service_az_load_balancer",
		"privatelink_service_vpc_endpoint",
		"billing_total",
		"billing_service",
		"billing_linked_account",
		"billing_linked_account_service",
	} {
		assert.Contains(t, profileIncludeHelp, profile)
	}
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics")["ui:listFlavour"])
	assert.Equal(t, "ec2", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics", "items", "profile")["ui:placeholder"])
	assert.Equal(t, "Average", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics", "items", "statistics", "items")["ui:placeholder"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics", "items", "include")["ui:listFlavour"])
	assert.Equal(t, "CPUUtilization", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics", "items", "include", "items", "name")["ui:placeholder"])
	assert.Equal(t, "Maximum", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "metrics", "items", "include", "items", "statistics", "items")["ui:placeholder"])
	assert.Equal(t, "us-east-1", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "regions", "items")["ui:placeholder"])
	assert.Equal(t, "5m", schemaObjectAt(t, doc, "uiSchema", "rules", "items", "query", "period")["ui:placeholder"])
	assert.Equal(t, "30m", schemaObjectAt(t, doc, "uiSchema", "rule_defaults", "query", "lookback")["ui:placeholder"])
	defaultLookbackHelp, ok := schemaObjectAt(t, doc, "uiSchema", "rule_defaults", "query", "lookback")["ui:help"].(string)
	require.True(t, ok)
	assert.Contains(t, defaultLookbackHelp, "old CloudWatch value")
	assert.Contains(t, defaultLookbackHelp, "transient")
	defaultDelayHelp, ok := schemaObjectAt(t, doc, "uiSchema", "rule_defaults", "query", "publication_delay")["ui:help"].(string)
	require.True(t, ok)
	assert.Contains(t, defaultDelayHelp, "overrides profile-specific delays")
	assert.Contains(t, defaultDelayHelp, "S3")
	assert.Contains(t, defaultDelayHelp, "1d")
	assert.Equal(t, "64", schemaObjectAt(t, doc, "uiSchema", "limits", "max_discovery_groups")["ui:placeholder"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "rule_defaults", "filters", "resource_tags")["ui:listFlavour"])
	assert.Equal(t, "Environment", schemaObjectAt(t, doc, "uiSchema", "rule_defaults", "filters", "resource_tags", "items", "key")["ui:placeholder"])
	assert.Equal(t, "list", schemaObjectAt(t, doc, "uiSchema", "labels", "resource_tags")["ui:listFlavour"])
	assert.Equal(t, "Owner", schemaObjectAt(t, doc, "uiSchema", "labels", "resource_tags", "items", "key")["ui:placeholder"])
	assert.NotEmpty(t, schemaObjectAt(t, doc, "uiSchema", "credentials")["ui:help"])
	recentlyActiveHelp, ok := schemaObjectAt(t, doc, "uiSchema", "discovery", "recently_active_only")["ui:help"].(string)
	require.True(t, ok)
	assert.Contains(t, recentlyActiveHelp, "publication_delay + lookback + period")
	assert.NotEmpty(t, schemaObjectAt(t, doc, "uiSchema", "vnode")["ui:placeholder"])
}

func TestMetadata_TwoRuleQueryPolicyExample(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	text := string(data)

	for _, want := range []string{
		"name: lambda-activity",
		"name: lambda-latency",
		"name: Invocations",
		"name: Duration",
		"period: 5m",
		"period: 1m",
	} {
		assert.Contains(t, text, want)
	}
}

func TestMetadata_BillingOperatorContract(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	text := string(data)

	for _, want := range []string{
		"billing_total",
		"billing_service",
		"billing_linked_account",
		"billing_linked_account_service",
		"resource_tags: []",
		"data collection cannot be disabled",
		"about 15 minutes",
		"management/payer account",
		"Amazon Partner Network (APN)",
		"UTC month boundary",
		"28,800 metric requests per day",
		"AWS charges `GetMetricData` by metrics requested",
		"Billing dimensions are not AWS Resource Groups Tagging API resources",
	} {
		assert.Contains(t, text, want)
	}
}

func metadataExampleConfig(t *testing.T, exampleName string) Config {
	t.Helper()
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var metadata struct {
		Modules []struct {
			Setup struct {
				Configuration struct {
					Examples struct {
						List []struct {
							Name   string `yaml:"name"`
							Config string `yaml:"config"`
						} `yaml:"list"`
					} `yaml:"examples"`
				} `yaml:"configuration"`
			} `yaml:"setup"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &metadata))
	for _, module := range metadata.Modules {
		for _, example := range module.Setup.Configuration.Examples.List {
			if example.Name != exampleName {
				continue
			}
			var configFile struct {
				Jobs []Config `yaml:"jobs"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(example.Config), &configFile))
			require.Len(t, configFile.Jobs, 1)
			return configFile.Jobs[0]
		}
	}
	require.FailNowf(t, "metadata example not found", "name=%q", exampleName)
	return Config{}
}

func TestMetadata_PrivateLinkEndpointOperatorContract(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	text := string(data)
	var metadata struct {
		Modules []struct {
			Meta struct {
				Keywords []string `yaml:"keywords"`
			} `yaml:"meta"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &metadata))
	require.Len(t, metadata.Modules, 1)
	assert.Subset(t, metadata.Modules[0].Meta.Keywords, []string{"privatelink", "private link", "vpc endpoint"})

	cfg := metadataExampleConfig(t, "AWS PrivateLink endpoints with split timing")
	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	require.Len(t, plan.Scopes, 3)
	wantScopes := []struct {
		profile string
		series  []string
		policy  cwquery.Policy
	}{
		{
			profile: "privatelink_endpoint",
			series: []string{
				"privatelink_endpoint.active_connections_average",
				"privatelink_endpoint.bytes_processed_average",
				"privatelink_endpoint.new_connections_average",
			},
			policy: cwquery.Policy{Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "privatelink_endpoint",
			series:  []string{"privatelink_endpoint.bytes_processed_sum"},
			policy:  cwquery.Policy{Period: 6 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "privatelink_endpoint_subnet",
			series: []string{
				"privatelink_endpoint_subnet.active_connections_average",
				"privatelink_endpoint_subnet.bytes_processed_average",
				"privatelink_endpoint_subnet.bytes_processed_sum",
				"privatelink_endpoint_subnet.new_connections_average",
				"privatelink_endpoint_subnet.new_connections_sum",
				"privatelink_endpoint_subnet.packets_dropped_sum",
				"privatelink_endpoint_subnet.rst_packets_received_sum",
			},
			policy: cwquery.Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		},
	}
	for i, want := range wantScopes {
		scope := plan.Scopes[i]
		assert.Equal(t, want.profile, scope.Profile.Name)
		assert.Equal(t, want.series, compiledSeriesNames(scope.SelectedSeries))
		for _, series := range scope.SelectedSeries {
			assert.Equal(t, want.policy, series.Policy, series.Name)
		}
	}

	for _, want := range []string{
		"privatelink_endpoint",
		"privatelink_endpoint_subnet",
		"AWS/PrivateLinkEndpoints",
		"AWS/PrivateLinkServices",
		"endpoint_type",
		"subnet_id",
		"seven additional metric/statistic queries",
		"1,440 metric requests per day",
	} {
		assert.Contains(t, text, want)
	}
}

func TestMetadata_PrivateLinkServiceOperatorContract(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	text := string(data)

	cfg := metadataExampleConfig(t, "AWS PrivateLink services with split timing")
	plan, diagnostics, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Empty(t, diagnostics)
	require.Len(t, plan.Scopes, 3)
	wantScopes := []struct {
		profile string
		series  []string
		policy  cwquery.Policy
	}{
		{
			profile: "privatelink_service",
			series: []string{
				"privatelink_service.active_connections_average",
				"privatelink_service.bytes_processed_average",
				"privatelink_service.new_connections_average",
				"privatelink_service.rst_packets_sent_average",
			},
			policy: cwquery.Policy{Period: time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "privatelink_service",
			series:  []string{"privatelink_service.endpoints_count_average"},
			policy:  cwquery.Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 5 * time.Minute},
		},
		{
			profile: "privatelink_service",
			series:  []string{"privatelink_service.bytes_processed_sum"},
			policy:  cwquery.Policy{Period: 6 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 5 * time.Minute},
		},
	}
	for i, want := range wantScopes {
		scope := plan.Scopes[i]
		assert.Equal(t, want.profile, scope.Profile.Name)
		assert.Equal(t, want.series, compiledSeriesNames(scope.SelectedSeries))
		for _, series := range scope.SelectedSeries {
			assert.Equal(t, want.policy, series.Policy, series.Name)
		}
	}

	for _, want := range []string{
		"privatelink_service",
		"privatelink_service_az",
		"privatelink_service_load_balancer",
		"privatelink_service_az_load_balancer",
		"privatelink_service_vpc_endpoint",
		"AWS/PrivateLinkServices",
		"ec2:vpc-endpoint-service",
		"1,152 metric requests per day",
	} {
		assert.Contains(t, text, want)
	}
}

func TestConfigSchema_QueryPolicyHasNoMaterializedDefaults(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.NotContains(t, string(data), "query_offset")

	ruleQuery := schemaObjectAt(t, doc, "jsonSchema", "properties", "rules", "items", "properties", "query", "properties")
	defaultQuery := schemaObjectAt(t, doc, "jsonSchema", "properties", "rule_defaults", "properties", "query", "properties")
	for _, field := range []string{"period", "lookback", "publication_delay"} {
		for _, query := range []map[string]any{ruleQuery, defaultQuery} {
			property := schemaObjectAt(t, query, field)
			assert.NotContainsf(t, property, "default", "%s must preserve omission for runtime precedence", field)
			assert.Equal(t, "string", property["type"])
		}
	}
}

func TestConfig_QueryPolicyPresenceAndCanonicalSerialization(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(`
rule_defaults:
  query:
    period: 5m
rules:
  - name: sample
    query:
      publication_delay: 0s
`), &cfg))
	require.NotNil(t, cfg.RuleDefaults.Query)
	require.NotNil(t, cfg.RuleDefaults.Query.Period)
	assert.Equal(t, 5*time.Minute, cfg.RuleDefaults.Query.Period.Duration())
	require.Len(t, cfg.Rules, 1)
	require.NotNil(t, cfg.Rules[0].Query)
	require.NotNil(t, cfg.Rules[0].Query.PublicationDelay)
	assert.Zero(t, cfg.Rules[0].Query.PublicationDelay.Duration())

	encoded, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "period: 5m")
	assert.Contains(t, string(encoded), "publication_delay: 0s")

	omitted, err := yaml.Marshal(Config{Rules: []RuleConfig{{Name: "sample"}}})
	require.NoError(t, err)
	assert.NotContains(t, string(omitted), "query:")
}

func TestConfigSchema_CredentialTypeSelector(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	source := schemaObjectAt(t, doc, "jsonSchema", "properties", "credentials", "items")
	assert.ElementsMatch(t, []string{"name", "type"}, schemaStringSlice(t, source["required"], "credential source required"))
	typeSchema := schemaObjectAt(t, source, "properties", "type")
	assert.Equal(t, []string{"default", "static"}, schemaStringSlice(t, typeSchema["enum"], "credential type enum"))
	assert.Equal(t, "default", typeSchema["default"])
	assert.Equal(t, "^[a-z][a-z0-9_-]{0,63}$", schemaObjectAt(t, source, "properties", "name")["pattern"])

	dependency := schemaObjectAt(t, source, "dependencies", "type")
	branches, ok := dependency["oneOf"].([]any)
	require.Truef(t, ok, "credential type dependency oneOf is %T", dependency["oneOf"])
	require.Len(t, branches, 2)
	staticBranch, ok := branches[1].(map[string]any)
	require.Truef(t, ok, "static credential branch is %T", branches[1])
	assert.Equal(t, []string{"type_static"}, schemaStringSlice(t, staticBranch["required"], "static branch required"))
	staticConfig := schemaObjectAt(t, staticBranch, "properties", "type_static")
	assert.ElementsMatch(t, []string{"access_key_id", "secret_access_key"},
		schemaStringSlice(t, staticConfig["required"], "static config required"))
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

func schemaStringSlice(t *testing.T, value any, path string) []string {
	t.Helper()
	raw, ok := value.([]any)
	require.Truef(t, ok, "%s is %T", path, value)
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		text, ok := item.(string)
		require.Truef(t, ok, "%s[%d] is %T", path, i, item)
		out = append(out, text)
	}
	return out
}

func TestConfigSchema_ValidationParity(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc struct {
		JSONSchema any `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("cloudwatch-config.json", doc.JSONSchema))
	schema, err := compiler.Compile("cloudwatch-config.json")
	require.NoError(t, err)

	base := map[string]any{
		"name":        "cloudwatch",
		"credentials": []any{map[string]any{"name": "sdk_default", "type": "default"}},
		"targets":     []any{map[string]any{"name": "base", "credentials": "sdk_default"}},
		"rules":       []any{map[string]any{"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"us-east-1"}}},
	}

	tests := map[string]struct {
		mutate         func(map[string]any)
		wantSchemaErr  bool
		wantRuntimeErr bool
	}{
		"base configuration": {},
		"exact metric selection": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{
					"name": "selected", "targets": []any{"base"}, "regions": []any{"us-east-1"},
					"profiles": map[string]any{"defaults": false, "include": []any{"ec2"}},
					"metrics": []any{map[string]any{
						"profile": "ec2", "statistics": []any{"Average"},
						"include": []any{map[string]any{"name": "CPUUtilization"}},
					}},
				}}
			},
		},
		"empty metric selection": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{
					"name": "selected", "targets": []any{"base"}, "regions": []any{"us-east-1"},
					"metrics": []any{},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"empty group statistics": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{
					"name": "selected", "targets": []any{"base"}, "regions": []any{"us-east-1"},
					"profiles": map[string]any{"defaults": false, "include": []any{"ec2"}},
					"metrics": []any{map[string]any{
						"profile": "ec2", "statistics": []any{},
						"include": []any{map[string]any{"name": "CPUUtilization", "statistics": []any{"Average"}}},
					}},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"empty metric statistics": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{
					"name": "selected", "targets": []any{"base"}, "regions": []any{"us-east-1"},
					"profiles": map[string]any{"defaults": false, "include": []any{"ec2"}},
					"metrics": []any{map[string]any{
						"profile": "ec2", "statistics": []any{"Average"},
						"include": []any{map[string]any{"name": "CPUUtilization", "statistics": []any{}}},
					}},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"resource tag filter inheritance and explicit disable": {
			mutate: func(cfg map[string]any) {
				cfg["rule_defaults"] = map[string]any{"filters": map[string]any{"resource_tags": []any{
					map[string]any{"key": "environment", "values": []any{"production", "staging"}},
				}}}
				cfg["labels"] = map[string]any{"resource_tags": []any{
					map[string]any{"key": "Owner", "label": "resource_owner"},
				}}
				cfg["limits"] = map[string]any{"max_instances": 2000, "max_discovery_groups": 80}
				cfg["rules"] = []any{
					map[string]any{"name": "filtered", "targets": []any{"base"}, "regions": []any{"us-east-1"}, "profiles": map[string]any{"defaults": false, "include": []any{"ec2"}}},
					map[string]any{"name": "unfiltered", "targets": []any{"base"}, "regions": []any{"us-east-1"}, "profiles": map[string]any{"defaults": false, "include": []any{"cloudfront"}}, "filters": map[string]any{"resource_tags": []any{}}},
				}
			},
		},
		"negative discovery group limit": {
			mutate:        func(cfg map[string]any) { cfg["limits"] = map[string]any{"max_discovery_groups": -1} },
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"discovery group limit above synchronous maximum": {
			mutate: func(cfg map[string]any) {
				cfg["limits"] = map[string]any{"max_discovery_groups": maxDiscoveryGroupsPerJob + 1}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"resource tag filter without values": {
			mutate: func(cfg map[string]any) {
				cfg["rule_defaults"] = map[string]any{"filters": map[string]any{"resource_tags": []any{
					map[string]any{"key": "environment", "values": []any{}},
				}}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"nested static credential source": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{
					"name": "static", "type": "static", "type_static": map[string]any{
						"access_key_id": "key", "secret_access_key": "secret", "session_token": "token",
					},
				}}
				cfg["targets"] = []any{map[string]any{"name": "base", "credentials": "static"}}
			},
		},
		"flat static fields": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{
					"name": "sdk_default", "type": "static", "access_key_id": "key", "secret_access_key": "secret",
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"credential map": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": "default"}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"duplicate credential names": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{
					map[string]any{"name": "sdk_default", "type": "default"},
					map[string]any{"name": "sdk_default", "type": "default"},
				}
			},
			wantRuntimeErr: true,
		},
		"credential without name": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{"type": "default"}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"credential type with surrounding whitespace": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{"name": "sdk_default", "type": " default "}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"mixed-case credential type": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{"name": "sdk_default", "type": "DEFAULT"}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"whitespace-only static access key": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{
					"name": "sdk_default", "type": "static", "type_static": map[string]any{
						"access_key_id": " ", "secret_access_key": "secret",
					},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"target name with surrounding whitespace": {
			mutate: func(cfg map[string]any) {
				cfg["targets"] = []any{map[string]any{"name": " base ", "credentials": "sdk_default"}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"target reference with surrounding whitespace": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{"name": "base-defaults", "targets": []any{" base "}, "regions": []any{"us-east-1"}}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"profile reference with surrounding whitespace": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{
					"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"us-east-1"},
					"profiles": map[string]any{"defaults": false, "include": []any{" ec2 "}},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"non-canonical region": {
			mutate: func(cfg map[string]any) {
				cfg["rules"] = []any{map[string]any{"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"US-EAST-1"}}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"missing required static field": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{
					"name": "sdk_default", "type": "static", "type_static": map[string]any{"access_key_id": "key"},
				}}
			},
			wantSchemaErr: true, wantRuntimeErr: true,
		},
		"unknown top-level field": {
			mutate: func(cfg map[string]any) { cfg["unknown"] = true },
		},
		"unknown nested field": {
			mutate: func(cfg map[string]any) {
				cfg["targets"] = []any{map[string]any{"name": "base", "credentials": "sdk_default", "unknown": true}}
			},
		},
		"inactive static configuration": {
			mutate: func(cfg map[string]any) {
				cfg["credentials"] = []any{map[string]any{
					"name": "sdk_default", "type": "default", "type_static": map[string]any{
						"access_key_id": "rejected-at-runtime", "secret_access_key": "secret",
					},
				}}
			},
			wantRuntimeErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := cloneConfigMap(t, base)
			if tc.mutate != nil {
				tc.mutate(cfg)
			}

			if tc.wantSchemaErr {
				assert.Error(t, schema.Validate(cfg))
			} else {
				assert.NoError(t, schema.Validate(cfg))
			}
			if tc.wantRuntimeErr {
				assert.Error(t, validateRuntimeConfigMap(t, cfg))
			} else {
				assert.NoError(t, validateRuntimeConfigMap(t, cfg))
			}
		})
	}
}

func cloneConfigMap(t *testing.T, src map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(src)
	require.NoError(t, err)
	var dst map[string]any
	require.NoError(t, json.Unmarshal(data, &dst))
	return dst
}

func validateRuntimeConfigMap(t *testing.T, raw map[string]any) error {
	t.Helper()
	data, err := yaml.Marshal(raw)
	require.NoError(t, err)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return err
	}
	_, _, err = compileTestConfig(t, cfg)
	return err
}

func TestConfig_ResourceTagLabelsDecode(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("labels:\n  resource_tags:\n    - key: owner\n    - key: Name\n      label: instance_name\n"), &cfg))
	assert.Equal(t, []ResourceTagLabelConfig{{Key: "owner"}, {Key: "Name", Label: "instance_name"}}, cfg.Labels.ResourceTags)
}

func TestConfig_MetricSelectorDecode(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("rules:\n  - metrics:\n      - profile: ec2\n        statistics: [Sum]\n        include:\n          - name: NetworkIn\n          - name: CPUUtilization\n            statistics: [Average]\n"), &cfg))
	require.Len(t, cfg.Rules, 1)
	assert.Equal(t, []ProfileMetricSelectorConfig{{
		Profile: "ec2", Statistics: []string{"Sum"},
		Include: []MetricSelectionConfig{{Name: "NetworkIn"}, {Name: "CPUUtilization", Statistics: []string{"Average"}}},
	}}, cfg.Rules[0].Metrics)
}

func TestConfig_TupleMetricSelectorIsNotCompatibilityDecoded(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("rules:\n  - metrics:\n      include:\n        - profile: ec2\n          metric: CPUUtilization\n          statistic: Average\n"), &cfg)
	assert.Error(t, err, "the discarded tuple grammar has no compatibility decoder")
}

func TestConfig_SeriesSelectorIsNotCompatibilityDecoded(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("rules:\n  - series:\n      include:\n        - profile: ec2\n          metric: CPUUtilization\n          statistic: Average\n"), &cfg))
	require.Len(t, cfg.Rules, 1)
	assert.Nil(t, cfg.Rules[0].Metrics, "the discarded draft name has no alias or compatibility decoder")
}

func TestConfig_LegacyTopLevelTagsAreNotDecoded(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("tags:\n  - name: owner\n    rename: resource_owner\n"), &cfg))
	assert.Empty(t, cfg.Labels.ResourceTags, "the removed prototype has no alias or compatibility decoder")
}

func TestConfig_ReplacedDefaultsKeyIsNotDecoded(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("defaults:\n  filters:\n    resource_tags:\n      - key: env\n        values: [prod]\n"), &cfg))
	assert.Empty(t, cfg.RuleDefaults.Filters.ResourceTags, "the replaced key has no alias or compatibility decoder")
}

func TestConfig_ResourceTagFilterInheritanceDecode(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("rule_defaults:\n  filters:\n    resource_tags:\n      - key: env\n        values: [prod]\nrules:\n  - name: inherit\n  - name: disable\n    filters:\n      resource_tags: []\n"), &cfg))
	require.Len(t, cfg.Rules, 2)
	assert.Nil(t, cfg.Rules[0].Filters)
	require.NotNil(t, cfg.Rules[1].Filters)
	require.NotNil(t, cfg.Rules[1].Filters.ResourceTags)
	assert.Empty(t, *cfg.Rules[1].Filters.ResourceTags)
	assert.Equal(t, cfg.RuleDefaults.Filters.ResourceTags, cfg.Rules[0].effectiveResourceTagFilters(cfg.RuleDefaults.Filters.ResourceTags))
	assert.Empty(t, cfg.Rules[1].effectiveResourceTagFilters(cfg.RuleDefaults.Filters.ResourceTags))
}

func TestNormalizeRegions(t *testing.T) {
	assert.Equal(t, []string{"us-east-1", "eu-west-1"},
		normalizeRegions([]string{"us-east-1", "us-east-1", "eu-west-1", "", "eu-west-1"}))
}
