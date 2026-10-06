// SPDX-License-Identifier: GPL-3.0-or-later

package rum_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func nativeRUMConfigFactory(t *testing.T) (*joboutput.ConfigModuleFactory, func() *rum.Collector) {
	t.Helper()
	db, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	creator := rum.Creator(rum.Dependencies{
		Registry: rumregistry.New(),
		History:  rumhistory.NewStore(db),
	})
	create := creator.CreateV2
	var last *rum.Collector
	creator.CreateV2 = func() collectorapi.CollectorV2 { last = create().(*rum.Collector); return last }
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	require.NoError(t, err)
	configs, err := secrets.NewConfigResolver(resolver, func([]string) (secretresolver.AtomicScope, error) {
		return nil, errors.New("no store references in this fixture")
	})
	require.NoError(t, err)
	factory, err := joboutput.NewConfigModuleFactory(
		joboutput.ConfigModuleFactoryConfig{
			Modules: collectorapi.Registry{
				"rum": creator,
			},
			Configs: configs,
		},
	)
	require.NoError(t, err)
	return factory, func() *rum.Collector { return last }
}

func nativeRUMConfig(t *testing.T, cfg rum.Config) confgroup.Config {
	t.Helper()
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	var out confgroup.Config
	require.NoError(t, yaml.Unmarshal(raw, &out))
	return out.SetModule("rum").SetSourceType(confgroup.TypeUser).SetSource("file=rum-test.conf").SetProvider("file")
}

func minimalRUMConfig() rum.Config {
	cfg := rum.New(rum.Dependencies{}).Config
	cfg.Name = "shop"
	cfg.AllowedOrigins = []string{"https://shop.example.org"}
	return cfg
}

func TestNativeRUMConfigurationNullDefaultsAndReload(t *testing.T) {
	for _, suffix := range []string{
		"", "event_logs: null\ntracing: null\n",
		"event_logs: {}\ntracing: {}\n",
		"event_logs: {enabled: null, include_console_logs: null, destination: null}\ntracing: {enabled: null, destination: null, propagate_to: null}\n",
		"event_logs: {destination: {endpoint: null, auth_token: null, tls_ca: null, tls_cert: null, tls_key: null}}\ntracing: {destination: {endpoint: null, auth_token: null, tls_ca: null, tls_cert: null, tls_key: null}}\n",
	} {
		t.Run(suffix, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			var input confgroup.Config
			require.NoError(
				t,
				yaml.Unmarshal([]byte("name: shop\nallowed_origins: [https://shop.example.org]\n"+suffix), &input),
			)
			input.SetModule("rum").SetSourceType(confgroup.TypeUser)
			payload, err := factory.Configuration(context.Background(), input)
			require.NoError(t, err)
			// Retrieval constructs and decodes the real collector but must not call Init.
			require.ErrorContains(t, last().Check(context.Background()), "not initialized")
			var effective rum.Config
			require.NoError(t, json.Unmarshal(payload, &effective))
			require.NotNil(t, effective.EventLogs)
			require.NotNil(t, effective.Tracing)
			assert.False(t, effective.EventLogsOn())
			assert.False(t, effective.TracingOn())
			assert.False(t, effective.ConsoleLogsOn())
			assert.Equal(t, config.DefaultEndpoint, *effective.EventLogs.Destination.Endpoint)
			assert.Equal(t, config.DefaultEndpoint, *effective.Tracing.Destination.Endpoint)
			assert.Empty(t, effective.EventLogs.Destination.AuthToken)
			assert.Empty(t, effective.Tracing.PropagateTo)
			assert.Contains(t, string(payload), `"enabled":false`)
			assert.Contains(t, string(payload), `"include_console_logs":false`)
			require.NoError(t, factory.Validate(context.Background(), input))
			require.NoError(t, factory.Test(context.Background(), input))
			assert.Equal(t, effective, last().Configuration())
			for _, format := range []string{"json", "yaml"} {
				var reload confgroup.Config
				if format == "json" {
					require.NoError(t, json.Unmarshal(payload, &reload))
				} else {
					reload = nativeRUMConfig(t, effective)
				}
				reload.SetModule("rum").SetSourceType(confgroup.TypeUser)
				again, err := factory.Configuration(context.Background(), reload)
				require.NoError(t, err)
				assert.JSONEq(t, string(payload), string(again))
				require.NoError(t, factory.Test(context.Background(), reload))
				assert.Equal(t, effective, last().Configuration())
			}
		})
	}
}

func TestNativeRUMActiveNullDestinationDefaults(t *testing.T) {
	for _, destination := range []string{"null", "{}", "{endpoint: null}"} {
		t.Run(destination, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			var input confgroup.Config
			raw := "name: shop\nallowed_origins: [https://shop.example.org]\nevent_logs: {enabled: true, destination: " + destination + "}\ntracing: {enabled: true, destination: " + destination + "}\n"
			require.NoError(t, yaml.Unmarshal([]byte(raw), &input))
			input.SetModule("rum").SetSourceType(confgroup.TypeUser)
			payload, err := factory.Configuration(context.Background(), input)
			require.NoError(t, err)
			var effective rum.Config
			require.NoError(t, json.Unmarshal(payload, &effective))
			assert.True(t, effective.EventLogsOn())
			assert.True(t, effective.TracingOn())
			assert.False(t, effective.ConsoleLogsOn())
			require.NotNil(t, effective.EventLogs.Destination.Endpoint)
			require.NotNil(t, effective.Tracing.Destination.Endpoint)
			assert.Equal(t, config.DefaultEndpoint, *effective.EventLogs.Destination.Endpoint)
			assert.Equal(t, config.DefaultEndpoint, *effective.Tracing.Destination.Endpoint)
			require.NoError(t, factory.Test(context.Background(), input))
			assert.Equal(t, effective, last().Configuration())
			again, err := factory.Configuration(context.Background(), nativeRUMConfig(t, effective))
			require.NoError(t, err)
			assert.JSONEq(t, string(payload), string(again))
		})
	}
}

func TestNativeRUMDestinationActivationContract(t *testing.T) {
	for _, signal := range []string{"event_logs", "tracing"} {
		t.Run(signal, func(t *testing.T) {
			factory, _ := nativeRUMConfigFactory(t)
			cfg := minimalRUMConfig()
			empty := ""
			if signal == "event_logs" {
				cfg.EventLogs = &config.EventLogs{
					Enabled: true,
					Destination: config.Destination{
						Endpoint: &empty,
					},
				}
			} else {
				cfg.Tracing = &config.Tracing{
					Enabled: true,
					Destination: config.Destination{
						Endpoint: &empty,
					},
				}
			}
			input := nativeRUMConfig(t, cfg)
			require.NoError(t, factory.Validate(context.Background(), input), "native admission is decode-only")
			payload, err := factory.Configuration(context.Background(), input)
			require.NoError(t, err)
			assert.Contains(t, string(payload), `"endpoint":""`)
			err = factory.Test(context.Background(), input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), signal+".destination")
			if signal == "event_logs" {
				cfg.EventLogs.Enabled = false
			} else {
				cfg.Tracing.Enabled = false
			}
			require.NoError(t, factory.Test(context.Background(), nativeRUMConfig(t, cfg)))
		})
	}
	factory, last := nativeRUMConfigFactory(t)
	cfg := minimalRUMConfig()
	badURL := "malformed://user:synthetic@receiver/path?token=synthetic"
	cfg.EventLogs = &config.EventLogs{
		IncludeConsoleLogs: true,
		Destination: config.Destination{
			Endpoint:  &badURL,
			TLSCA:     "/does-not-exist/ca",
			TLSCert:   "/does-not-exist/cert",
			TLSKey:    "/does-not-exist/key",
			AuthToken: "saved-log-token",
		},
	}
	cfg.Tracing = &config.Tracing{
		Destination: config.Destination{
			Endpoint:  &badURL,
			TLSCA:     "/does-not-exist/trace-ca",
			TLSCert:   "unpaired",
			AuthToken: "saved-trace-token",
		},
	}
	input := nativeRUMConfig(t, cfg)
	require.NoError(t, factory.Validate(context.Background(), input))
	require.NoError(t, factory.Test(context.Background(), input))
	assert.Equal(t, cfg, last().Configuration(), "inactive settings remain available for later editing")
	payload, err := factory.Configuration(context.Background(), input)
	require.NoError(t, err)
	var restored rum.Config
	require.NoError(t, json.Unmarshal(payload, &restored))
	assert.Equal(t, cfg, restored)
}

func TestNativeRUMIndependentDestinationsAndSecretResolution(t *testing.T) {
	const logsEnv = "NETDATA_DEM_TEST_CONFIG_LOGS_TOKEN"
	const tracesEnv = "NETDATA_DEM_TEST_CONFIG_TRACES_TOKEN"
	const missingEnv = "NETDATA_DEM_TEST_CONFIG_ABSENT_TOKEN_78AF0499"
	t.Setenv(logsEnv, "synthetic-logs-secret-value")
	t.Setenv(tracesEnv, "synthetic-traces-secret-value")
	// Setenv registers restoration; Unsetenv exercises the actual missing-env provider.
	t.Setenv(missingEnv, "")
	require.NoError(t, os.Unsetenv(missingEnv))
	factory, last := nativeRUMConfigFactory(t)
	cfg := minimalRUMConfig()
	logsURL, tracesURL := "http://127.0.0.1:1", "https://127.0.0.1:2"
	cfg.EventLogs = &config.EventLogs{
		Enabled:            true,
		IncludeConsoleLogs: true,
		Destination: config.Destination{
			Endpoint:  &logsURL,
			AuthToken: "${env:" + logsEnv + "}",
		},
	}
	cfg.Tracing = &config.Tracing{
		Enabled:     true,
		PropagateTo: []string{"https://api.example.org"},
		Destination: config.Destination{
			Endpoint:  &tracesURL,
			AuthToken: "${env:" + tracesEnv + "}",
		},
	}
	input := nativeRUMConfig(t, cfg)
	payload, err := factory.Configuration(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "${env:"+logsEnv+"}")
	assert.NotContains(t, string(payload), "synthetic-logs-secret-value")
	require.NoError(t, factory.Validate(context.Background(), input))
	require.NoError(
		t,
		factory.Test(context.Background(), input),
		"Test validates locally without probing either receiver",
	)
	assert.Equal(t, logsURL, *last().EventLogs.Destination.Endpoint)
	assert.Equal(t, tracesURL, *last().Tracing.Destination.Endpoint)
	assert.Equal(t, "synthetic-logs-secret-value", last().EventLogs.Destination.AuthToken)
	assert.Equal(t, "synthetic-traces-secret-value", last().Tracing.Destination.AuthToken)
	cfg.EventLogs.Enabled = false
	cfg.Tracing.Enabled = false
	cfg.Tracing.Destination.AuthToken = "${env:" + missingEnv + "}"
	input = nativeRUMConfig(t, cfg)
	require.NoError(t, factory.Validate(context.Background(), input), "Validate does not resolve inactive references")
	_, err = factory.Configuration(context.Background(), input)
	require.NoError(t, err, "Configuration preserves the unresolved reference")
	err = factory.Test(context.Background(), input)
	require.Error(t, err, "native activation resolves the entire job, including inactive destinations")
	assert.NotContains(t, err.Error(), "synthetic-logs-secret-value")
	cfg.EventLogs.Enabled = true
	malformed := "https://user:synthetic-logs-secret-value@receiver.invalid/path"
	cfg.EventLogs.Destination.Endpoint = &malformed
	cfg.Tracing.Destination.AuthToken = "${env:" + tracesEnv + "}"
	input = nativeRUMConfig(t, cfg)
	require.NoError(t, factory.Validate(context.Background(), input))
	err = factory.Test(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redacted")
	assert.NotContains(t, err.Error(), "synthetic-logs-secret-value")
	assert.NotContains(t, err.Error(), malformed)
}

func TestNativeRUMSamplingConfigurationAndReload(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		rate         float64
		keep         bool
	}{
		{"omitted", "", 1, true},
		{"null", "measure_sample_rate: null\ninvestigate: null\n", 1, true},
		{"empty", "investigate: {}\n", 1, true},
		{"null fields", "measure_sample_rate: null\ninvestigate: {sample_rate: null, always_keep: null}\n", 1, true},
		{"zero", "measure_sample_rate: 0\ninvestigate: {sample_rate: 0}\n", 0, true},
		{"zero no overrides", "measure_sample_rate: 0\ninvestigate: {sample_rate: 0, always_keep: []}\n", 0, false},
		{"fraction", "measure_sample_rate: 0.25\ninvestigate: {sample_rate: 0.25}\n", .25, true},
		{"one", "measure_sample_rate: 1\ninvestigate: {sample_rate: 1}\n", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			var input confgroup.Config
			require.NoError(
				t,
				yaml.Unmarshal([]byte("name: shop\nallowed_origins: [https://shop.example.org]\n"+tc.suffix), &input),
			)
			input.SetModule("rum").SetSourceType(confgroup.TypeUser)
			payload, err := factory.Configuration(context.Background(), input)
			require.NoError(t, err)
			require.ErrorContains(t, last().Check(context.Background()), "not initialized")
			var effective rum.Config
			require.NoError(t, json.Unmarshal(payload, &effective))
			require.NotNil(t, effective.MeasureSampleRate)
			require.NotNil(t, effective.Investigate)
			require.NotNil(t, effective.Investigate.SampleRate)
			assert.Equal(t, tc.rate, *effective.MeasureSampleRate)
			assert.Equal(t, tc.rate, *effective.Investigate.SampleRate)
			assert.Equal(t, tc.keep, effective.KeepsErrors())
			assert.Equal(t, tc.keep, effective.KeepsPoorVitals())
			require.NoError(t, factory.Test(context.Background(), input))
			assert.Equal(t, effective, last().Configuration())
			for _, format := range []string{"json", "yaml"} {
				var reload confgroup.Config
				if format == "json" {
					require.NoError(t, json.Unmarshal(payload, &reload))
				} else {
					reload = nativeRUMConfig(t, effective)
				}
				reload.SetModule("rum").SetSourceType(confgroup.TypeUser)
				again, err := factory.Configuration(context.Background(), reload)
				require.NoError(t, err)
				assert.JSONEq(t, string(payload), string(again))
				require.NoError(t, factory.Test(context.Background(), reload))
				assert.Equal(t, effective, last().Configuration())
			}
		})
	}
}

func TestNativeRUMCaptureConfigurationAndReload(t *testing.T) {
	for _, tc := range []struct {
		suffix, mode string
		frustration  bool
	}{
		{"", config.GeolocationCountry, false},
		{"capture: null", config.GeolocationCountry, false},
		{"capture: {}", config.GeolocationCountry, false},
		{"capture: {geolocation: null, frustration_signals: null}", config.GeolocationCountry, false},
		{"capture: {geolocation: 'off'}", config.GeolocationOff, false},
		{"capture: {geolocation: city, frustration_signals: true}", config.GeolocationCity, true},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			factory, last := nativeRUMConfigFactory(t)
			var input confgroup.Config
			require.NoError(t, yaml.Unmarshal([]byte("name: shop\nallowed_origins: [https://shop.example.org]\n"+tc.suffix), &input))
			input.SetModule("rum").SetSourceType(confgroup.TypeUser)
			payload, err := factory.Configuration(context.Background(), input)
			require.NoError(t, err)
			require.ErrorContains(t, last().Check(context.Background()), "not initialized")
			var effective rum.Config
			require.NoError(t, json.Unmarshal(payload, &effective))
			require.NotNil(t, effective.Capture)
			require.NotNil(t, effective.Capture.Geolocation)
			assert.Equal(t, tc.mode, effective.GeolocationMode())
			assert.Equal(t, tc.frustration, effective.FrustrationSignalsOn())
			require.NoError(t, factory.Test(context.Background(), input))
			assert.Equal(t, effective, last().Configuration())
			again, err := factory.Configuration(context.Background(), nativeRUMConfig(t, effective))
			require.NoError(t, err)
			assert.JSONEq(t, string(payload), string(again))
		})
	}
}

func TestCaptureFrustrationChartsFollowPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			site, hub, _ := contractSite(t)
			site.Capture.FrustrationSignals = enabled
			recv := receiver.New(hub)
			recv.Listen = "127.0.0.1:0"
			startJob(t, "receiver", "receiver", recv)
			job, out, stop := startJob(t, "rum", "shop", site)
			sendContractBeacon(t, hub, []byte(`{"meta":{"page":{"id":"cart-document","url":"https://example.org/cart"},"session":{"id":"capture-session"}},"events":[{"name":"document_activated","attributes":{"observation_id":"cart-document","observation_sequence":"1"}},{"name":"rage_click"}]}`))
			tickUntil(t, job, out, "SET 'document_views' = 1")
			stop()
			if enabled {
				assert.Contains(t, out.String(), "rum.frustration")
				assert.Contains(t, out.String(), "SET 'rage_clicks' = 1")
			} else {
				assert.NotContains(t, out.String(), "rum.frustration")
				assert.NotContains(t, out.String(), "SET 'rage_clicks'")
			}
		})
	}
}
