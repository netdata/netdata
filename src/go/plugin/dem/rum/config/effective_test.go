// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestEffectiveFeatureDefaultsAndRoundTrip(t *testing.T) {
	for name, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(name, func(t *testing.T) {
			for _, raw := range []string{`{}`, `{"event_logs":null,"tracing":null}`, `{"event_logs":{"enabled":null,"include_console_logs":null,"destination":null},"tracing":{"enabled":null,"destination":{"endpoint":null}}}`} {
				var s Site
				require.NoError(t, decode([]byte(raw), &s))
				effective := s.Effective()
				require.False(t, effective.EventLogsOn())
				require.False(t, effective.TracingOn())
				require.False(t, effective.ConsoleLogsOn())
				require.Equal(t, DefaultEndpoint, *effective.EventLogs.Destination.Endpoint)
				require.Equal(t, DefaultEndpoint, *effective.Tracing.Destination.Endpoint)
				if s.EventLogs != nil {
					require.Nil(t, s.EventLogs.Destination.Endpoint)
				}
				if s.Tracing != nil {
					require.Nil(t, s.Tracing.Destination.Endpoint)
				}
				encoded, err := json.Marshal(effective)
				require.NoError(t, err)
				require.Contains(t, string(encoded), `"enabled":false`)
				var round Site
				require.NoError(t, json.Unmarshal(encoded, &round))
				require.Equal(t, effective, round.Effective())
			}
		})
	}
}

func TestEffectiveFeaturesRemainIndependent(t *testing.T) {
	for _, logs := range []bool{false, true} {
		for _, traces := range []bool{false, true} {
			empty := ""
			s := Site{
				EventLogs: &EventLogs{
					Enabled:            logs,
					IncludeConsoleLogs: true,
					Destination: Destination{
						Endpoint:  &empty,
						AuthToken: "log-token",
						TLSCA:     "retained-missing.pem",
					},
				},
				Tracing: &Tracing{
					Enabled: traces,
					Destination: Destination{
						AuthToken: "trace-token",
					},
				},
			}
			e := s.Effective()
			require.Equal(t, logs, e.EventLogsOn())
			require.Equal(t, traces, e.TracingOn())
			require.Equal(t, logs, e.ConsoleLogsOn())
			require.Equal(t, "", *e.EventLogs.Destination.Endpoint)
			require.Equal(t, DefaultEndpoint, *e.Tracing.Destination.Endpoint)
			if logs {
				require.NotEmpty(t, ValidateSiteExtras(e))
			} else {
				require.Empty(t, ValidateSiteExtras(e))
			}
			*e.EventLogs.Destination.Endpoint = "http://changed:4317"
			e.EventLogs.Enabled = !logs
			require.Equal(t, "", *s.EventLogs.Destination.Endpoint)
			require.Equal(t, logs, s.EventLogs.Enabled)
			require.Equal(t, "log-token", s.EventLogs.Destination.AuthToken)
			require.Equal(t, "trace-token", s.Tracing.Destination.AuthToken)
		}
	}
}

func TestDisabledDestinationsRetainButDoNotValidate(t *testing.T) {
	s := Site{
		EventLogs: &EventLogs{
			Destination: Destination{
				Endpoint: stringPtr("not a URL"),
				TLSCert:  "unpaired",
			},
		},
		Tracing: &Tracing{
			PropagateTo: []string{"not an origin"},
			Destination: Destination{
				Endpoint: stringPtr(""),
				TLSCA:    "missing",
			},
		},
	}
	require.Empty(t, ValidateSiteExtras(s))
	s.EventLogs.Enabled = true
	require.NotEmpty(t, ValidateSiteExtras(s))
	s.EventLogs.Enabled = false
	s.Tracing.Enabled = true
	require.NotEmpty(t, ValidateSiteExtras(s))
	data, err := yaml.Marshal(s)
	require.NoError(t, err)
	var round Site
	require.NoError(t, yaml.Unmarshal(data, &round))
	require.Equal(t, s.EventLogs, round.EventLogs)
	require.Equal(t, s.Tracing, round.Tracing)
}
func stringPtr(s string) *string { return &s }
