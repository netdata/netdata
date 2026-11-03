// SPDX-License-Identifier: GPL-3.0-or-later
package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestCaptureDefaultsAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		raw, mode   string
		frustration bool
	}{
		{`{}`, GeolocationCountry, false},
		{`{"capture":null}`, GeolocationCountry, false},
		{`{"capture":{}}`, GeolocationCountry, false},
		{`{"capture":{"geolocation":null,"frustration_signals":null}}`, GeolocationCountry, false},
		{`{"capture":{"geolocation":"off"}}`, GeolocationOff, false},
		{`{"capture":{"geolocation":"country","frustration_signals":true}}`, GeolocationCountry, true},
		{`{"capture":{"geolocation":"city","frustration_signals":true}}`, GeolocationCity, true},
	} {
		for format, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
			t.Run(format+tc.raw, func(t *testing.T) {
				site := Site{AllowedOrigins: []string{"https://example.org"}}
				require.NoError(t, decode([]byte(tc.raw), &site))
				before, err := json.Marshal(site)
				require.NoError(t, err)
				effective := site.Effective()
				after, err := json.Marshal(site)
				require.NoError(t, err)
				require.Equal(t, before, after)
				for _, candidate := range []Site{site, effective} {
					require.Equal(t, tc.mode, candidate.GeolocationMode())
					require.Equal(t, tc.frustration, candidate.FrustrationSignalsOn())
					require.Empty(t, ValidateSiteExtras(candidate))
					for _, codec := range []struct {
						marshal   func(any) ([]byte, error)
						unmarshal func([]byte, any) error
					}{{json.Marshal, json.Unmarshal}, {yaml.Marshal, yaml.Unmarshal}} {
						data, err := codec.marshal(candidate)
						require.NoError(t, err)
						var restored Site
						require.NoError(t, codec.unmarshal(data, &restored))
						require.Equal(t, candidate, restored)
					}
				}
				*effective.Capture.Geolocation = "changed"
				effective.Capture.FrustrationSignals = !tc.frustration
				require.Equal(t, tc.mode, site.GeolocationMode())
				require.Equal(t, tc.frustration, site.FrustrationSignalsOn())
			})
		}
	}
}
