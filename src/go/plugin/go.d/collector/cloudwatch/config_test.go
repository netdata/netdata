// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validBaseConfig() Config {
	return Config{
		UpdateEvery: 60,
		Regions:     []string{"us-east-1"},
		Auth:        awsauth.Config{Mode: awsauth.ModeDefault},
		Profiles:    ProfilesConfig{Mode: profilesModeAuto},
		Discovery:   DiscoveryConfig{RefreshEvery: 300},
		QueryOffset: 600,
		Timeout:     defaultTimeout,
	}
}

func TestConfig_validate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(c *Config)
		wantErr bool
	}{
		"valid":                       {mutate: func(*Config) {}},
		"no regions":                  {mutate: func(c *Config) { c.Regions = nil }, wantErr: true},
		"mixed partitions":            {mutate: func(c *Config) { c.Regions = []string{"us-east-1", "cn-north-1"} }, wantErr: true},
		"single gov partition is ok":  {mutate: func(c *Config) { c.Regions = []string{"us-gov-east-1", "us-gov-west-1"} }},
		"update_every below minimum":  {mutate: func(c *Config) { c.UpdateEvery = 30 }, wantErr: true},
		"refresh_every below minimum": {mutate: func(c *Config) { c.Discovery.RefreshEvery = 30 }, wantErr: true},
		"negative query_offset":       {mutate: func(c *Config) { c.QueryOffset = -1 }, wantErr: true},
		"negative timeout":            {mutate: func(c *Config) { c.Timeout = confopt.Duration(-time.Second) }, wantErr: true},
		"exact mode without entries":  {mutate: func(c *Config) { c.Profiles = ProfilesConfig{Mode: profilesModeExact} }, wantErr: true},
		"unsupported profiles mode":   {mutate: func(c *Config) { c.Profiles.Mode = "weird" }, wantErr: true},
		"invalid auth mode":           {mutate: func(c *Config) { c.Auth.Mode = "bogus" }, wantErr: true},
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

func TestConfig_validate_regionCaseHiddenPartition(t *testing.T) {
	// An uppercase region must not slip past the mixed-partition guard: regions are
	// lowercased before partition detection, so us-east-1 + CN-NORTH-1 is still a
	// cross-partition (aws + aws-cn) job and must be rejected.
	cfg := validBaseConfig()
	cfg.Regions = []string{"us-east-1", "CN-NORTH-1"}
	assert.Error(t, cfg.validate())
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

	assert.ElementsMatch(t, []string{"regions", "auth"}, doc.JSONSchema.Required)

	// Every public Config key must have a schema property (drift guard).
	for _, key := range []string{
		"update_every", "autodetection_retry", "vnode", "regions", "auth", "profiles",
		"discovery", "query_offset", "timeout",
	} {
		assert.Contains(t, doc.JSONSchema.Properties, key, "schema property for %q", key)
	}

	// Credential fields must be marked sensitive (secret_access_key, session_token, external_id).
	assert.Equal(t, 3, strings.Count(string(data), `"sensitive": true`), "credential fields marked sensitive")
}

func TestRegionPartition(t *testing.T) {
	tests := map[string]string{
		"us-east-1":       "aws",
		"eu-west-3":       "aws",
		"cn-north-1":      "aws-cn",
		"cn-northwest-1":  "aws-cn",
		"us-gov-east-1":   "aws-us-gov",
		"us-gov-west-1":   "aws-us-gov",
		"us-iso-east-1":   "aws-iso",
		"us-isob-east-1":  "aws-iso-b",
		"us-isof-south-1": "aws-iso-f",
		"eu-isoe-west-1":  "aws-iso-e",
		"eusc-de-east-1":  "aws-eusc",
	}
	for region, want := range tests {
		t.Run(region, func(t *testing.T) {
			assert.Equal(t, want, regionPartition(region))
		})
	}
}

func TestConfig_Regions(t *testing.T) {
	c := Config{Regions: []string{"us-east-1", " us-east-1 ", "US-EAST-1", "EU-West-1", "eu-west-1", "us-east-1", ""}}
	assert.Equal(t, []string{"us-east-1", "eu-west-1"}, c.regions(),
		"regions are lowercased, trimmed, de-duplicated (first wins), and empties dropped")
}
