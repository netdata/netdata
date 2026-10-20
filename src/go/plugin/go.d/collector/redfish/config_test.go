// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"maps"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

const allTrimSpaceCharacters = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

func TestCollectorConfigurationSerialize(t *testing.T) {
	require.NotEmpty(t, dataConfigJSON)
	require.NotEmpty(t, dataConfigYAML)
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestConfigSchemaAuthenticationMatchesRuntime(t *testing.T) {
	schema := compileConfigSchema(t)
	base := func() map[string]any {
		return map[string]any{"url": "https://bmc.example.test"}
	}
	with := func(values map[string]any) map[string]any {
		config := base()
		maps.Copy(config, values)
		return config
	}

	tests := map[string]struct {
		config map[string]any
		valid  bool
	}{
		"none without credential keys": {
			config: with(map[string]any{"auth_method": "none"}), valid: true,
		},
		"none with runtime-empty credentials": {
			config: with(map[string]any{"auth_method": "none", "username": " \t ", "password": ""}), valid: true,
		},
		"none with Unicode runtime-empty username": {
			config: with(map[string]any{"auth_method": "none", "username": "\v\u00a0", "password": ""}), valid: true,
		},
		"none with every runtime-empty username character": {
			config: with(map[string]any{"auth_method": "none", "username": allTrimSpaceCharacters, "password": ""}),
			valid:  true,
		},
		"auto with credentials": {
			config: with(map[string]any{"auth_method": "auto", "username": "user", "password": "secret"}), valid: true,
		},
		"session with credentials": {
			config: with(
				map[string]any{"auth_method": "session", "username": "user", "password": "secret"},
			), valid: true,
		},
		"basic with credentials": {
			config: with(map[string]any{"auth_method": "basic", "username": "user", "password": "secret"}), valid: true,
		},
		"basic preserves whitespace password": {
			config: with(map[string]any{"auth_method": "basic", "username": " user ", "password": " "}), valid: true,
		},
		"omitted method defaults to auto": {
			config: with(map[string]any{"username": "user", "password": "secret"}), valid: true,
		},
		"omitted method and credentials": {config: base()},
		"none with username": {
			config: with(map[string]any{"auth_method": "none", "username": "user"}),
		},
		"none with password": {
			config: with(map[string]any{"auth_method": "none", "password": "secret"}),
		},
		"none with whitespace password": {
			config: with(map[string]any{"auth_method": "none", "password": " "}),
		},
		"auto missing password": {
			config: with(map[string]any{"auth_method": "auto", "username": "user"}),
		},
		"auto with empty credentials": {
			config: with(map[string]any{"auth_method": "auto", "username": "", "password": ""}),
		},
		"session missing username": {
			config: with(map[string]any{"auth_method": "session", "password": "secret"}),
		},
		"session with whitespace username": {
			config: with(map[string]any{"auth_method": "session", "username": " \t ", "password": "secret"}),
		},
		"session with vertical-tab username": {
			config: with(map[string]any{"auth_method": "session", "username": "\v", "password": "secret"}),
		},
		"session with no-break-space username": {
			config: with(map[string]any{"auth_method": "session", "username": "\u00a0", "password": "secret"}),
		},
		"session with every runtime-empty username character": {
			config: with(map[string]any{
				"auth_method": "session", "username": allTrimSpaceCharacters, "password": "secret",
			}),
		},
		"basic missing credentials": {
			config: with(map[string]any{"auth_method": "basic"}),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schemaErr := schema.Validate(test.config)
			raw, err := json.Marshal(test.config)
			require.NoError(t, err)
			var cfg Config
			require.NoError(t, json.Unmarshal(raw, &cfg))
			cfg.applyDefaults()
			runtimeErr := cfg.validate()
			if test.valid {
				require.NoError(t, schemaErr)
				require.NoError(t, runtimeErr)
			} else {
				require.Error(t, schemaErr)
				require.Error(t, runtimeErr)
			}
		})
	}
}

func TestConfigSchemaTLSClientIdentityMatchesRuntime(t *testing.T) {
	schema := compileConfigSchema(t)
	base := func() map[string]any {
		return map[string]any{
			"url": "https://bmc.example.test", "auth_method": "none",
		}
	}
	with := func(values map[string]any) map[string]any {
		config := base()
		maps.Copy(config, values)
		return config
	}

	tests := map[string]struct {
		config map[string]any
		valid  bool
	}{
		"both absent": {config: base(), valid: true},
		"both empty": {
			config: with(map[string]any{"tls_cert": "", "tls_key": ""}),
			valid:  true,
		},
		"empty certificate alone": {config: with(map[string]any{"tls_cert": ""}), valid: true},
		"empty key alone":         {config: with(map[string]any{"tls_key": ""}), valid: true},
		"both runtime-empty": {
			config: with(map[string]any{"tls_cert": " \t", "tls_key": "\v\u00a0"}),
			valid:  true,
		},
		"both every runtime-empty character": {
			config: with(map[string]any{"tls_cert": allTrimSpaceCharacters, "tls_key": allTrimSpaceCharacters}),
			valid:  true,
		},
		"both configured": {
			config: with(map[string]any{"tls_cert": "certificate", "tls_key": "key"}),
			valid:  true,
		},
		"both configured with whitespace": {
			config: with(map[string]any{"tls_cert": " certificate ", "tls_key": "\u00a0key\u3000"}),
			valid:  true,
		},
		"certificate only": {config: with(map[string]any{"tls_cert": "certificate"})},
		"key only":         {config: with(map[string]any{"tls_key": "key"})},
		"certificate with runtime-empty key": {
			config: with(map[string]any{"tls_cert": "certificate", "tls_key": allTrimSpaceCharacters}),
		},
		"runtime-empty certificate with key": {
			config: with(map[string]any{"tls_cert": allTrimSpaceCharacters, "tls_key": "key"}),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schemaErr := schema.Validate(test.config)
			raw, err := json.Marshal(test.config)
			require.NoError(t, err)
			var cfg Config
			require.NoError(t, json.Unmarshal(raw, &cfg))
			cfg.applyDefaults()
			runtimeErr := cfg.validate()
			if test.valid {
				require.NoError(t, schemaErr)
				require.NoError(t, runtimeErr)
			} else {
				require.Error(t, schemaErr)
				require.Error(t, runtimeErr)
			}
		})
	}
}

func compileConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var document struct {
		JSONSchema any `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("config-schema.json", document.JSONSchema))
	schema, err := compiler.Compile("config-schema.json")
	require.NoError(t, err)
	return schema
}

func TestConfigValidation(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"valid HTTPS no auth": {
			cfg: Config{
				URL:        "https://BMC.EXAMPLE.TEST:443/redfish/v1/",
				AuthMethod: "none",
			},
		},
		"valid explicit HTTP": {
			cfg: Config{
				URL:        "http://bmc.example.test",
				AuthMethod: "none",
			},
		},
		"credentials required": {
			cfg: Config{
				URL:        "https://bmc.example.test",
				AuthMethod: "session",
			},
			wantErr: "'username' and 'password'",
		},
		"none rejects credentials": {
			cfg: Config{
				URL:        "https://bmc.example.test",
				AuthMethod: "none",
				Username:   "user",
			},
			wantErr: "must be empty",
		},
		"link local requires zone": {
			cfg: Config{
				URL:        "https://[fe80::1]/",
				AuthMethod: "none",
			},
			wantErr: "requires an interface zone",
		},
		"malformed URL does not disclose user-info": {
			cfg: Config{
				URL:        "https://monitor:test-password@%zz/redfish/v1/",
				AuthMethod: "none",
			},
			wantErr: "invalid URL syntax",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.cfg.applyDefaults()
			err := test.cfg.validate()
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
				require.NotContains(t, err.Error(), "test-password")
			}
		})
	}
}
