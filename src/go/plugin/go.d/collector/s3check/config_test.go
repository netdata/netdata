// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestCollectorConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestConfigSchemaMatchesMetadataGroups(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestConfigSchemaAcceptsLifecycleAndReplication(t *testing.T) {
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
		"source": map[string]any{
			"region": "us-east-1",
			"bucket": "probe-bucket",
		},
	}))
}

func TestConfigDefaultsUseHumanDurations(t *testing.T) {
	cfg := Config{Name: "job"}
	cfg.applyDefaults()

	assert.Equal(t, string(contract.ModeLifecycle), cfg.Mode)
	assert.Equal(t, defaultPrefix, cfg.Prefix)
	assert.Equal(t, defaultWriteObjective, cfg.WriteObjective.Duration())
	assert.Equal(t, defaultWriteTimeout, cfg.WriteTimeout.Duration())
	assert.Equal(t, defaultDeleteObjective, cfg.DeleteObjective.Duration())
	assert.Equal(t, defaultDeleteTimeout, cfg.DeleteTimeout.Duration())
	assert.Equal(t, awsauth.CredentialTypeDefault, cfg.Source.Credentials.Type)
	assert.True(t, boolValue(cfg.Source.PathStyle))
}

func TestConfigValidationModeBoundaries(t *testing.T) {
	t.Run("lifecycle rejects destination", func(t *testing.T) {
		cfg := validConfig(contract.ModeLifecycle)
		cfg.Destination = &S3Config{}
		cfg.applyDefaults()
		assert.ErrorContains(t, cfg.validate(), "destination is only valid")
	})

	for _, mode := range []contract.Mode{contract.ModeCephMultisite, contract.ModeAWSReplication} {
		t.Run(string(mode)+" requires destination", func(t *testing.T) {
			cfg := validConfig(mode)
			cfg.Destination = nil
			assert.ErrorContains(t, cfg.validate(), "destination is required")
		})
	}

	t.Run("replication objectives use scheduler-scale durations", func(t *testing.T) {
		cfg := validConfig(contract.ModeCephMultisite)
		cfg.WriteObjective = confopt.LongDuration(time.Second)
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
			cfg.Source.Endpoint = tc.sourceEndpoint
			cfg.Source.Bucket = "shared-bucket"
			cfg.Source.PathStyle = new(tc.sourcePathStyle)
			cfg.Destination.Endpoint = tc.destinationEndpoint
			cfg.Destination.Bucket = "shared-bucket"
			cfg.Destination.PathStyle = new(tc.destinationPathStyle)

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
	assert.Contains(t, guide, "write_objective:")
	assert.Contains(t, guide, "delete_timeout:")
	assert.NotContains(t, guide, "mode: multisite")
	assert.NotContains(t, guide, "rpo_threshold_ms")
	assert.NotContains(t, guide, "replication_timeout_ms")
	assert.NotContains(t, guide, "verify_delete")
}

func TestOwnershipFingerprintContract(t *testing.T) {
	cfg := validConfig(contract.ModeAWSReplication)
	want := cfg.ownershipFingerprint()

	presentationAndTransport := cfg
	presentationAndTransport.Source.Name = "renamed source"
	presentationAndTransport.Source.Region = "eu-west-1"
	presentationAndTransport.Source.Credentials = awsauth.CredentialConfig{
		Type: awsauth.CredentialTypeStatic,
		TypeStatic: &awsauth.StaticCredentialConfig{
			AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "secret",
		},
	}
	presentationAndTransport.Source.Timeout = confopt.LongDuration(20 * time.Second)
	presentationAndTransport.Source.ProxyURL = "http://proxy.example"
	presentationAndTransport.Source.PathStyle = new(false)
	presentationAndTransport.Destination = cloneS3Config(cfg.Destination)
	presentationAndTransport.Destination.Name = "renamed destination"
	presentationAndTransport.Destination.Region = "eu-central-1"
	assert.Equal(t, want, presentationAndTransport.ownershipFingerprint())

	location := cfg
	location.Destination = cloneS3Config(cfg.Destination)
	location.Destination.Bucket = "another-destination"
	assert.NotEqual(t, want, location.ownershipFingerprint())

	mapping := cfg
	mapping.Prefix = "other-prefix/"
	assert.NotEqual(t, want, mapping.ownershipFingerprint())
}

func validConfig(mode contract.Mode) Config {
	cfg := New().Config
	cfg.Name = "s3check-test"
	cfg.Mode = string(mode)
	cfg.Source.Endpoint = "https://source.example"
	cfg.Source.Region = "us-east-1"
	cfg.Source.Bucket = "source-bucket"
	if modeUsesDestination(cfg.Mode) {
		cfg.Destination = &S3Config{
			Name: "destination", Endpoint: "https://destination.example",
			Region: "us-east-1", Bucket: "destination-bucket",
		}
		cfg.Destination.applyDefaults("destination")
	}
	return cfg
}

func cloneS3Config(value *S3Config) *S3Config {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
