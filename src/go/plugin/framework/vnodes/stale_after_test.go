// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestStaleAfterLabels(t *testing.T) {
	for name, tc := range map[string]struct {
		value *confopt.Duration
		want  map[string]string
	}{
		"omitted retains legacy label":       {want: map[string]string{"site": "athens", "_node_stale_after_seconds": "90"}},
		"explicit duration overrides legacy": {value: durationPointer(5 * time.Minute), want: map[string]string{"site": "athens", "_node_stale_after_seconds": "300"}},
		"zero disables legacy timeout":       {value: durationPointer(0), want: map[string]string{"site": "athens"}},
	} {
		t.Run(name, func(t *testing.T) {
			v := VirtualNode{
				StaleAfter: tc.value,
				Labels:     map[string]string{"site": "athens", "_node_stale_after_seconds": "90"},
			}
			original := v.Copy()
			assert.Equal(t, tc.want, v.HostLabels())
			assert.Equal(t, original, &v)
			copy := v.Copy()
			copy.Labels["site"] = "london"
			if copy.StaleAfter != nil {
				*copy.StaleAfter = confopt.Duration(time.Hour)
			}
			assert.Equal(t, original, &v)
		})
	}
}

func TestStaleAfterValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		duration time.Duration
		valid    bool
	}{
		"zero": {valid: true}, "minute": {duration: time.Minute, valid: true},
		"negative": {duration: -time.Second}, "fractional second": {duration: 1500 * time.Millisecond},
		"uint32 boundary": {duration: time.Duration(^uint32(0)) * time.Second, valid: true},
		"uint32 overflow": {duration: (time.Duration(^uint32(0)) + 1) * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			v := VirtualNode{
				Name:       "router",
				Hostname:   "router",
				GUID:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				SourceType: "user",
				StaleAfter: durationPointer(tc.duration),
			}
			err := ValidateConfigured(&v)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, "stale_after")
			}
		})
	}
}

func TestStaleAfterConfigurationRoundTrip(t *testing.T) {
	for name, tc := range map[string]struct {
		json, yaml string
		want       *confopt.Duration
	}{
		"omission":       {json: `{}`, yaml: `{}`},
		"null":           {json: `{"stale_after":null}`, yaml: `stale_after: null`},
		"zero":           {json: `{"stale_after":0}`, yaml: `stale_after: 0`, want: durationPointer(0)},
		"human duration": {json: `{"stale_after":"5m"}`, yaml: `stale_after: 5m`, want: durationPointer(5 * time.Minute)},
	} {
		t.Run(name, func(t *testing.T) {
			for format, codec := range map[string]struct {
				input  string
				decode func([]byte, any) error
				encode func(any) ([]byte, error)
			}{
				"json": {input: tc.json, decode: json.Unmarshal, encode: json.Marshal}, "yaml": {input: tc.yaml, decode: yaml.Unmarshal, encode: yaml.Marshal},
			} {
				t.Run(format, func(t *testing.T) {
					var vnode VirtualNode
					require.NoError(t, codec.decode([]byte(codec.input), &vnode))
					assert.Equal(t, tc.want, vnode.StaleAfter)
					encoded, err := codec.encode(vnode)
					require.NoError(t, err)
					var roundTrip VirtualNode
					require.NoError(t, codec.decode(encoded, &roundTrip))
					assert.Equal(t, vnode, roundTrip)
				})
			}
		})
	}
}

func durationPointer(value time.Duration) *confopt.Duration {
	duration := confopt.Duration(value)
	return &duration
}
