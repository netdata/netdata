// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestCollectorConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollectorConfigurationDecodesIntoNew(t *testing.T) {
	c := New()
	require.NoError(t, yaml.Unmarshal(dataConfigYAML, c))
	c.Config.applyDefaults()

	require.NoError(t, c.Config.validate())
	assert.Nil(t, c.ModeLifecycle)
	assert.Nil(t, c.ModeCephMultisite)
	require.NotNil(t, c.ModeAWSReplication)
}

func TestCollectorConfigurationReturnsEffectiveSelectedMode(t *testing.T) {
	c := New()
	require.NoError(t, yaml.Unmarshal([]byte(`
name: aws-replication
mode: aws_replication
mode_aws_replication:
  source:
    region: us-east-1
    bucket: source-bucket
  destination:
    region: us-west-2
    bucket: destination-bucket
`), c))

	config, ok := c.Configuration().(Config)
	require.True(t, ok)
	assert.Nil(t, config.ModeLifecycle)
	assert.Nil(t, config.ModeCephMultisite)
	require.NotNil(t, config.ModeAWSReplication)
	assert.Equal(t, defaultPrefix, config.ModeAWSReplication.Prefix)
	assert.Equal(t, defaultWriteObjective, config.ModeAWSReplication.WriteObjective.Duration())
	assert.Equal(t, defaultWriteTimeout, config.ModeAWSReplication.WriteTimeout.Duration())
	assert.Equal(t, defaultDeleteObjective, config.ModeAWSReplication.DeleteObjective.Duration())
	assert.Equal(t, defaultDeleteTimeout, config.ModeAWSReplication.DeleteTimeout.Duration())
	assert.Equal(t, "source", config.ModeAWSReplication.Source.Name)
	assert.Equal(t, "destination", config.ModeAWSReplication.Destination.Name)
	assert.Nil(t, config.ModeAWSReplication.Source.Credentials)
	assert.True(t, boolValue(config.ModeAWSReplication.Source.PathStyle))
	assert.Equal(t, defaultTimeout, config.ModeAWSReplication.Source.Timeout.Duration())

	assert.Empty(t, c.ModeAWSReplication.Prefix, "rendering effective config must not mutate the collector")
}

func TestConfigSchemaMatchesMetadataGroups(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestConfigSchemaModeBranches(t *testing.T) {
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("s3check.schema.json", document["jsonSchema"]))
	schema, err := compiler.Compile("s3check.schema.json")
	require.NoError(t, err)

	var replication any
	require.NoError(t, json.Unmarshal(dataConfigJSON, &replication))
	require.NoError(t, schema.Validate(replication))
	require.NoError(t, schema.Validate(map[string]any{
		"mode": "lifecycle",
		"mode_lifecycle": map[string]any{
			"prefix": "netdata-s3check/",
			"source": map[string]any{
				"region": "us-east-1",
				"bucket": "probe-bucket",
				"credentials": map[string]any{
					"access_key_id":     "key",
					"secret_access_key": "secret",
				},
			},
		},
	}))
	require.NoError(t, schema.Validate(map[string]any{
		"mode": "ceph_multisite",
		"mode_ceph_multisite": map[string]any{
			"prefix":      "netdata-s3check/",
			"source":      map[string]any{"region": "us-east-1", "bucket": "source-bucket"},
			"destination": map[string]any{"region": "us-east-1", "bucket": "destination-bucket"},
		},
	}))
	require.Error(t, schema.Validate(map[string]any{
		"mode": "lifecycle",
		"mode_lifecycle": map[string]any{
			"source": map[string]any{"region": "us-east-1", "bucket": "probe-bucket"},
		},
		"source": map[string]any{"region": "us-east-1", "bucket": "legacy-bucket"},
	}))
	require.Error(t, schema.Validate(map[string]any{
		"mode": "lifecycle",
		"mode_lifecycle": map[string]any{
			"source": map[string]any{"region": "us-east-1", "bucket": "probe-bucket"},
		},
		"mode_aws_replication": map[string]any{
			"source":      map[string]any{"region": "us-east-1", "bucket": "source-bucket"},
			"destination": map[string]any{"region": "us-west-2", "bucket": "destination-bucket"},
		},
	}))
	require.Error(t, schema.Validate(map[string]any{
		"mode": "lifecycle",
		"mode_lifecycle": map[string]any{
			"source": map[string]any{
				"region":      "us-east-1",
				"bucket":      "probe-bucket",
				"credentials": map[string]any{"type": "default"},
			},
		},
	}))
}

func TestConfigSchemaSensitiveFieldsUsePasswordWidgets(t *testing.T) {
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &document))

	modeEndpoints := map[string][]string{
		"mode_lifecycle":       {"source"},
		"mode_ceph_multisite":  {"source", "destination"},
		"mode_aws_replication": {"source", "destination"},
	}
	for mode, endpoints := range modeEndpoints {
		for _, endpoint := range endpoints {
			t.Run(mode+"/"+endpoint, func(t *testing.T) {
				for _, field := range []string{"access_key_id", "secret_access_key", "session_token"} {
					assert.Equal(t, "password", configSchemaValueAt(t, document,
						"uiSchema", mode, endpoint, "credentials", field, "ui:widget"))
				}
				assert.Equal(t, "password", configSchemaValueAt(t, document,
					"uiSchema", mode, endpoint, "assume_role", "external_id", "ui:widget"))
				assert.Equal(t, "password", configSchemaValueAt(t, document,
					"uiSchema", mode, endpoint, "proxy_url", "ui:widget"))
			})
		}
	}
}

func configSchemaValueAt(t *testing.T, root map[string]any, path ...string) any {
	t.Helper()
	var value any = root
	for _, key := range path {
		object, ok := value.(map[string]any)
		require.Truef(t, ok, "%q is not an object", key)
		value, ok = object[key]
		require.Truef(t, ok, "missing schema path component %q", key)
	}
	return value
}

func TestConfigDefaultsUseHumanDurations(t *testing.T) {
	lifecycle := Config{Name: "job"}
	lifecycle.applyDefaults()

	require.NotNil(t, lifecycle.ModeLifecycle)
	assert.Equal(t, string(contract.ModeLifecycle), lifecycle.Mode)
	assert.Equal(t, defaultPrefix, lifecycle.ModeLifecycle.Prefix)
	assert.Nil(t, lifecycle.ModeLifecycle.Source.Credentials)
	assert.True(t, boolValue(lifecycle.ModeLifecycle.Source.PathStyle))

	replication := Config{Name: "job", Mode: string(contract.ModeCephMultisite)}
	replication.applyDefaults()
	require.NotNil(t, replication.ModeCephMultisite)
	assert.Equal(t, defaultPrefix, replication.ModeCephMultisite.Prefix)
	assert.Equal(t, defaultWriteObjective, replication.ModeCephMultisite.WriteObjective.Duration())
	assert.Equal(t, defaultWriteTimeout, replication.ModeCephMultisite.WriteTimeout.Duration())
	assert.Equal(t, defaultDeleteObjective, replication.ModeCephMultisite.DeleteObjective.Duration())
	assert.Equal(t, defaultDeleteTimeout, replication.ModeCephMultisite.DeleteTimeout.Duration())
	assert.Nil(t, replication.ModeCephMultisite.Destination.Credentials)
}

func TestS3ConfigCredentialSelection(t *testing.T) {
	defaultChain := S3Config{}.credentialConfig()
	assert.Equal(t, awsauth.CredentialTypeDefault, defaultChain.Type)
	assert.Nil(t, defaultChain.TypeStatic)

	static := &awsauth.StaticCredentialConfig{
		AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "token",
	}
	explicit := S3Config{Credentials: static}.credentialConfig()
	assert.Equal(t, awsauth.CredentialTypeStatic, explicit.Type)
	assert.Same(t, static, explicit.TypeStatic)
}

func TestS3ConfigStaticCredentialValidationUsesPublicPath(t *testing.T) {
	cfg := validConfig(contract.ModeLifecycle)
	cfg.ModeLifecycle.Source.Credentials = &awsauth.StaticCredentialConfig{AccessKeyID: "key"}

	err := cfg.validate()
	assert.ErrorContains(t, err, "source.credentials.secret_access_key is required")
	assert.NotContains(t, err.Error(), "type_static")
}

func TestConfigValidationModeBoundaries(t *testing.T) {
	t.Run("connection name matches schema limit", func(t *testing.T) {
		cfg := validConfig(contract.ModeLifecycle)
		cfg.ModeLifecycle.Source.Name = strings.Repeat("x", 65)
		assert.ErrorContains(t, cfg.validate(), "source.name must not exceed 64 characters")
	})

	t.Run("lifecycle rejects non-selected mode config", func(t *testing.T) {
		cfg := validConfig(contract.ModeLifecycle)
		cfg.ModeCephMultisite = &ReplicationModeConfig{}
		cfg.applyDefaults()
		assert.ErrorContains(t, cfg.validate(), "mode_ceph_multisite is only valid")
	})

	for _, mode := range []contract.Mode{contract.ModeCephMultisite, contract.ModeAWSReplication} {
		t.Run(string(mode)+" requires selected config", func(t *testing.T) {
			cfg := validConfig(mode)
			if mode == contract.ModeCephMultisite {
				cfg.ModeCephMultisite = nil
			} else {
				cfg.ModeAWSReplication = nil
			}
			assert.ErrorContains(t, cfg.validate(), "is required when mode is")
		})
	}

	t.Run("replication objectives use scheduler-scale durations", func(t *testing.T) {
		cfg := validConfig(contract.ModeCephMultisite)
		cfg.ModeCephMultisite.WriteObjective = confopt.LongDuration(time.Second)
		assert.ErrorContains(t, cfg.validate(), "write_objective")
	})
}

func TestConfigRejectsEquivalentS3Locations(t *testing.T) {
	tests := map[string]struct {
		sourceEndpoint       string
		destinationEndpoint  string
		sourcePathStyle      bool
		destinationPathStyle bool
	}{
		"default port": {
			sourceEndpoint: "http://127.0.0.1", destinationEndpoint: "http://127.0.0.1:80",
			sourcePathStyle: true, destinationPathStyle: true,
		},
		"hostname case and root dot": {
			sourceEndpoint: "https://RGW.EXAMPLE.", destinationEndpoint: "https://rgw.example:443",
			sourcePathStyle: true, destinationPathStyle: true,
		},
		"equivalent IPv6 literal": {
			sourceEndpoint: "http://[::1]", destinationEndpoint: "http://[0:0:0:0:0:0:0:1]:80",
			sourcePathStyle: true, destinationPathStyle: true,
		},
		"virtual host and path style": {
			sourceEndpoint: "https://shared-bucket.rgw.example", destinationEndpoint: "https://rgw.example:443",
			sourcePathStyle: false, destinationPathStyle: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig(contract.ModeCephMultisite)
			mode := cfg.ModeCephMultisite
			mode.Source.Endpoint = tc.sourceEndpoint
			mode.Source.Bucket = "shared-bucket"
			mode.Source.PathStyle = new(tc.sourcePathStyle)
			mode.Destination.Endpoint = tc.destinationEndpoint
			mode.Destination.Bucket = "shared-bucket"
			mode.Destination.PathStyle = new(tc.destinationPathStyle)

			assert.ErrorContains(t, cfg.validate(), "source and destination must identify different S3 locations")
		})
	}
}

func TestCephGuideUsesCurrentS3CheckContract(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "..", "docs", "guides", "ceph", "README.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	guide := string(raw)

	assert.Contains(t, guide, "mode: ceph_multisite")
	assert.Contains(t, guide, "mode_ceph_multisite:")
	assert.Contains(t, guide, "write_objective:")
	assert.Contains(t, guide, "delete_timeout:")
	assert.NotContains(t, guide, "mode: multisite")
	assert.NotContains(t, guide, "rpo_threshold_ms")
	assert.NotContains(t, guide, "replication_timeout_ms")
	assert.NotContains(t, guide, "verify_delete")
}

func TestOwnershipFingerprintContract(t *testing.T) {
	cfg := validConfig(contract.ModeAWSReplication)
	want := mustSelectedModeConfig(t, &cfg).ownershipFingerprint()

	presentationAndTransport := cloneConfig(cfg)
	presentationAndTransport.ModeAWSReplication.Source.Name = "renamed source"
	presentationAndTransport.ModeAWSReplication.Source.Region = "eu-west-1"
	presentationAndTransport.ModeAWSReplication.Source.Credentials = &awsauth.StaticCredentialConfig{
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
	}
	presentationAndTransport.ModeAWSReplication.Source.Timeout = confopt.LongDuration(20 * time.Second)
	presentationAndTransport.ModeAWSReplication.Source.ProxyURL = "http://proxy.example"
	presentationAndTransport.ModeAWSReplication.Source.PathStyle = new(false)
	presentationAndTransport.ModeAWSReplication.Destination.Name = "renamed destination"
	presentationAndTransport.ModeAWSReplication.Destination.Region = "eu-central-1"
	assert.Equal(t, want, mustSelectedModeConfig(t, &presentationAndTransport).ownershipFingerprint())

	location := cloneConfig(cfg)
	location.ModeAWSReplication.Destination.Bucket = "another-destination"
	assert.NotEqual(t, want, mustSelectedModeConfig(t, &location).ownershipFingerprint())

	mapping := cloneConfig(cfg)
	mapping.ModeAWSReplication.Prefix = "other-prefix/"
	assert.NotEqual(t, want, mustSelectedModeConfig(t, &mapping).ownershipFingerprint())
}

func validConfig(mode contract.Mode) Config {
	cfg := Config{Name: "s3check-test", Mode: string(mode)}
	cfg.applyDefaults()
	source := S3Config{Endpoint: "https://source.example", Region: "us-east-1", Bucket: "source-bucket"}
	source.applyDefaults("source")
	switch mode {
	case contract.ModeLifecycle:
		cfg.ModeLifecycle.Source = source
	case contract.ModeCephMultisite, contract.ModeAWSReplication:
		destination := S3Config{
			Name: "destination", Endpoint: "https://destination.example",
			Region: "us-east-1", Bucket: "destination-bucket",
		}
		destination.applyDefaults("destination")
		selected := cfg.ModeCephMultisite
		if mode == contract.ModeAWSReplication {
			selected = cfg.ModeAWSReplication
		}
		selected.Source = source
		selected.Destination = destination
	}
	return cfg
}

func mustSelectedModeConfig(t *testing.T, config *Config) *selectedModeConfig {
	t.Helper()
	selected, err := config.selectedModeConfig()
	require.NoError(t, err)
	return selected
}

func cloneConfig(value Config) Config {
	cloned := value
	if value.ModeLifecycle != nil {
		mode := *value.ModeLifecycle
		cloned.ModeLifecycle = &mode
	}
	if value.ModeCephMultisite != nil {
		mode := *value.ModeCephMultisite
		cloned.ModeCephMultisite = &mode
	}
	if value.ModeAWSReplication != nil {
		mode := *value.ModeAWSReplication
		cloned.ModeAWSReplication = &mode
	}
	return cloned
}
