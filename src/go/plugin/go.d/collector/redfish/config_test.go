// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
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

func TestConfigSchemaHostScopeOverridesRequireSystemVnodes(t *testing.T) {
	schema := compileConfigSchema(t)

	valid := []any{
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
			"host_scope_overrides": nil,
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
			"host_scope_overrides": []any{},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "/redfish/v1/Systems/1", "hostname": "system-one",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": " /redfish/v1/Systems/1 ", "guid": "", "hostname": " system-one ",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "/redfish/v1/Systems/1", "guid": " 550e8400-e29b-41d4-a716-446655440000 ",
				"hostname": "",
			}},
		},
	}
	for _, config := range valid {
		require.NoError(t, schema.Validate(config))
	}

	invalid := []any{
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "/redfish/v1/Systems/1", "hostname": "system-one",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "   ", "hostname": "system-one",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "/redfish/v1/Systems/1", "hostname": "\t",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "\v", "hostname": "system-one",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": "/redfish/v1/Systems/1", "hostname": "\u00a0",
			}},
		},
		map[string]any{
			"url": "https://bmc.example.test", "node_mode": "system_vnodes", "auth_method": "none",
			"host_scope_overrides": []any{map[string]any{
				"resource_uri": allTrimSpaceCharacters, "hostname": "system-one",
			}},
		},
	}
	for _, config := range invalid {
		require.Error(t, schema.Validate(config))
	}
}

func TestConfigSchemaAuthenticationMatchesRuntime(t *testing.T) {
	schema := compileConfigSchema(t)
	base := func() map[string]any {
		return map[string]any{"url": "https://bmc.example.test", "node_mode": "local"}
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
			config: with(map[string]any{"auth_method": "session", "username": "user", "password": "secret"}), valid: true,
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
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
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
		"both absent":                        {config: base(), valid: true},
		"both empty":                         {config: with(map[string]any{"tls_cert": "", "tls_key": ""}), valid: true},
		"empty certificate alone":            {config: with(map[string]any{"tls_cert": ""}), valid: true},
		"empty key alone":                    {config: with(map[string]any{"tls_key": ""}), valid: true},
		"both runtime-empty":                 {config: with(map[string]any{"tls_cert": " \t", "tls_key": "\v\u00a0"}), valid: true},
		"both every runtime-empty character": {config: with(map[string]any{"tls_cert": allTrimSpaceCharacters, "tls_key": allTrimSpaceCharacters}), valid: true},
		"both configured":                    {config: with(map[string]any{"tls_cert": "certificate", "tls_key": "key"}), valid: true},
		"both configured with whitespace":    {config: with(map[string]any{"tls_cert": " certificate ", "tls_key": "\u00a0key\u3000"}), valid: true},
		"certificate only":                   {config: with(map[string]any{"tls_cert": "certificate"})},
		"key only":                           {config: with(map[string]any{"tls_key": "key"})},
		"certificate with runtime-empty key": {config: with(map[string]any{"tls_cert": "certificate", "tls_key": allTrimSpaceCharacters})},
		"runtime-empty certificate with key": {config: with(map[string]any{"tls_cert": allTrimSpaceCharacters, "tls_key": "key"})},
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

func TestConfigSchemaLogsBackendMatchesRuntime(t *testing.T) {
	schema := compileConfigSchema(t)
	base := func() map[string]any {
		return map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
		}
	}
	withBackend := func(value string) map[string]any {
		config := base()
		config["logs"] = map[string]any{"backend": value}
		return config
	}

	tests := map[string]struct {
		config map[string]any
		valid  bool
	}{
		"omitted":                       {config: base(), valid: true},
		"configured":                    {config: withBackend("isolated"), valid: true},
		"configured with whitespace":    {config: withBackend(" \u00a0isolated\u3000 "), valid: true},
		"maximum ASCII bytes":           {config: withBackend(strings.Repeat("x", 256)), valid: true},
		"empty":                         {config: withBackend("")},
		"ASCII whitespace only":         {config: withBackend(" \t\r\n")},
		"every runtime-empty character": {config: withBackend(allTrimSpaceCharacters)},
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

func TestConfigSchemaLeavesLogsBackendByteLimitToRuntime(t *testing.T) {
	schema := compileConfigSchema(t)
	base := func(value string) map[string]any {
		return map[string]any{
			"url": "https://bmc.example.test", "node_mode": "local", "auth_method": "none",
			"logs": map[string]any{"backend": value},
		}
	}

	tests := map[string]struct {
		config       map[string]any
		runtimeValid bool
	}{
		"257 ASCII bytes": {
			config: base(strings.Repeat("x", 257)),
		},
		"258 multibyte UTF-8 bytes": {
			config: base(strings.Repeat("é", 129)),
		},
		"256 normalized bytes with surrounding whitespace": {
			config: base(" \u00a0" + strings.Repeat("x", 256) + "\u3000 "), runtimeValid: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Draft-07 maxLength counts pre-normalization code points, not normalized UTF-8 bytes.
			require.NoError(t, schema.Validate(test.config))
			raw, err := json.Marshal(test.config)
			require.NoError(t, err)
			var cfg Config
			require.NoError(t, json.Unmarshal(raw, &cfg))
			cfg.applyDefaults()
			runtimeErr := cfg.validate()
			if test.runtimeValid {
				require.NoError(t, runtimeErr)
				require.Len(t, cfg.LogsBackend(), 256)
			} else {
				require.ErrorContains(t, runtimeErr, "must not exceed 256 bytes")
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

func TestConfigDefaultsPreserveExplicitZeroSemantics(t *testing.T) {
	zero := 0
	zeroDuration := confopt.Duration(0)
	cfg := Config{
		URL:        "https://bmc.example.test",
		NodeMode:   "local",
		AuthMethod: "none",
		Retries:    &zero,
		Charts: ChartsConfig{
			MaxDetailedComponentsPerFamily: &zero,
		},
		Logs: LogsConfig{
			FullReconciliationEvery: &zeroDuration,
			Cursor: CursorConfig{
				OrphanRetention: &zeroDuration,
			},
		},
	}

	cfg.applyDefaults()

	require.Zero(t, *cfg.Retries)
	require.Zero(t, *cfg.Charts.MaxDetailedComponentsPerFamily)
	require.Zero(t, cfg.Logs.FullReconciliationEvery.Duration())
	require.Zero(t, cfg.Logs.Cursor.OrphanRetention.Duration())
	require.Equal(t, "default", cfg.LogsBackend())
	require.NoError(t, cfg.validate())

	explicit := "isolated"
	cfg.Logs.Backend = &explicit
	require.Equal(t, "isolated", cfg.LogsBackend())
}

func TestConfigValidation(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"valid HTTPS no auth": {
			cfg: Config{URL: "https://BMC.EXAMPLE.TEST:443/redfish/v1/", NodeMode: "local", AuthMethod: "none"},
		},
		"valid explicit HTTP": {
			cfg: Config{URL: "http://bmc.example.test", NodeMode: "system_vnodes", AuthMethod: "none"},
		},
		"local mode rejects HostScope overrides": {
			cfg: Config{
				URL: "https://bmc.example.test", NodeMode: "local", AuthMethod: "none",
				HostScopeOverrides: []HostScopeOverride{{
					ResourceURI: "/redfish/v1/Systems/1",
					Hostname:    "system-one",
				}},
			},
			wantErr: "'host_scope_overrides' requires node_mode \"system_vnodes\"",
		},
		"node mode required": {
			cfg:     Config{URL: "https://bmc.example.test", AuthMethod: "none"},
			wantErr: "'node_mode'",
		},
		"credentials required": {
			cfg:     Config{URL: "https://bmc.example.test", NodeMode: "local", AuthMethod: "session"},
			wantErr: "'username' and 'password'",
		},
		"none rejects credentials": {
			cfg: Config{
				URL: "https://bmc.example.test", NodeMode: "local", AuthMethod: "none", Username: "user",
			},
			wantErr: "must be empty",
		},
		"link local requires zone": {
			cfg:     Config{URL: "https://[fe80::1]/", NodeMode: "local", AuthMethod: "none"},
			wantErr: "requires an interface zone",
		},
		"explicit empty backend rejected": {
			cfg: Config{
				URL: "https://bmc.example.test", NodeMode: "local", AuthMethod: "none",
				Logs: LogsConfig{Backend: new("")},
			},
			wantErr: "must not be explicitly empty",
		},
		"malformed URL does not disclose user-info": {
			cfg: Config{
				URL:        "https://monitor:test-password@%zz/redfish/v1/",
				NodeMode:   "local",
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

func TestNormalizeServiceRoot(t *testing.T) {
	root, origin, err := normalizeServiceRoot("https://BMC.Example.TEST.:443/redfish/v1")
	require.NoError(t, err)
	require.Equal(t, "https://bmc.example.test", origin)
	require.Equal(t, "https://bmc.example.test/redfish/v1/", root.String())

	root, origin, err = normalizeServiceRoot("https://[fe80::1%25eno1]:443/")
	require.NoError(t, err)
	require.Equal(t, "https://[fe80::1%25eno1]", origin)
	require.Equal(t, "https://[fe80::1%25eno1]/redfish/v1/", root.String())

	root, origin, err = normalizeServiceRoot("http://BMC.Example.TEST:080/redfish/v1/")
	require.NoError(t, err)
	require.Equal(t, "http://bmc.example.test", origin)
	require.Equal(t, "http://bmc.example.test/redfish/v1/", root.String())

	_, _, err = normalizeServiceRoot("https://./redfish/v1/")
	require.ErrorContains(t, err, "host is required")

	_, _, err = normalizeServiceRoot("https://bmc%25suffix/redfish/v1/")
	require.ErrorContains(t, err, "DNS host must not contain a percent escape")

	_, _, err = normalizeServiceRoot("https://[::ffff:192.0.2.1%25eth0]/redfish/v1/")
	require.ErrorContains(t, err, "IPv4 host must not contain an interface zone")
}

func TestNormalizeServiceRootCanonicalizesIPv4MappedIPv6(t *testing.T) {
	plainRoot, plainOrigin, err := normalizeServiceRoot("https://192.0.2.1/redfish/v1/")
	require.NoError(t, err)
	mappedRoot, mappedOrigin, err := normalizeServiceRoot("https://[::ffff:192.0.2.1]/redfish/v1/")
	require.NoError(t, err)
	require.Equal(t, plainOrigin, mappedOrigin)
	require.Equal(t, plainRoot.String(), mappedRoot.String())

	plainURL, plainKey, err := DiscoveryEndpointIdentity("https://192.0.2.1")
	require.NoError(t, err)
	mappedURL, mappedKey, err := DiscoveryEndpointIdentity("https://[::ffff:192.0.2.1]")
	require.NoError(t, err)
	require.Equal(t, plainURL, mappedURL)
	require.Equal(t, plainKey, mappedKey)
}

func TestApplyDefaultsCanonicalizesHostScopeOverrideGUID(t *testing.T) {
	const canonical = "00000000-0000-4000-8000-000000000010"
	for _, raw := range []string{
		"00000000000040008000000000000010",
		"00000000-0000-4000-8000-000000000010",
		"URN:UUID:00000000-0000-4000-8000-000000000010",
	} {
		t.Run(raw, func(t *testing.T) {
			cfg := Config{HostScopeOverrides: []HostScopeOverride{{GUID: raw}}}
			cfg.applyDefaults()
			require.Equal(t, canonical, cfg.HostScopeOverrides[0].GUID)
		})
	}
}
