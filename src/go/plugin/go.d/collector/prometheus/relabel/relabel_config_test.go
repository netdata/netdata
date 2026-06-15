// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"encoding/json"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

func TestConfig_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		doc     string
		want    Config
		wantErr bool
	}{
		"full rule": {
			doc: `
source_labels: [code, method]
separator: "@"
regex: "(.+)@(.+)"
target_label: route
replacement: "$1-$2"
action: replace
`,
			want: Config{
				SourceLabels:    []string{"code", "method"},
				Separator:       "@",
				Regex:           MustNewRegexp("(.+)@(.+)"),
				TargetLabel:     "route",
				Replacement:     "$1-$2",
				Action:          Replace,
				NameScheme:      defaultNameValidationScheme,
				sourceLabelsSet: true,
				separatorSet:    true,
				replacementSet:  true,
			},
		},
		"minimal rule applies defaults, sets no flags": {
			doc: "action: drop\nregex: \"go_.*\"",
			want: Config{
				Separator:   ";",
				Regex:       MustNewRegexp("go_.*"),
				Replacement: "$1",
				Action:      Drop,
				NameScheme:  defaultNameValidationScheme,
			},
		},
		"explicit empty separator is preserved": {
			doc: "separator: \"\"\naction: replace\ntarget_label: x",
			want: Config{
				Separator:    "",
				Regex:        MustNewRegexp("(.*)"),
				Replacement:  "$1",
				Action:       Replace,
				TargetLabel:  "x",
				NameScheme:   defaultNameValidationScheme,
				separatorSet: true,
			},
		},
		"action is canonicalized": {doc: "action: DROP", want: configWithAction(t, Drop)},
		"invalid action rejected": {doc: "action: nope", wantErr: true},
		"invalid regex rejected":  {doc: `regex: "([)"`, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got Config
			err := yaml.Unmarshal([]byte(tc.doc), &got)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assertConfigEqual(t, tc.want, got)
		})
	}
}

// TestConfig_roundTrip proves a rule survives marshal -> unmarshal for both YAML and
// JSON (the regex and action keep their string form, so dyncfg serialization is stable).
func TestConfig_roundTrip(t *testing.T) {
	in := Config{
		SourceLabels: []string{"a", "b"},
		Separator:    "@",
		Regex:        MustNewRegexp("(.+)"),
		TargetLabel:  "t",
		Replacement:  "$1",
		Action:       Replace,
	}

	t.Run("yaml", func(t *testing.T) {
		b, err := yaml.Marshal(in)
		require.NoError(t, err)
		var out Config
		require.NoError(t, yaml.Unmarshal(b, &out))
		assert.Equal(t, []string{"a", "b"}, out.SourceLabels)
		assert.Equal(t, "@", out.Separator)
		assert.Equal(t, "(.+)", out.Regex.String())
		assert.Equal(t, "t", out.TargetLabel)
		assert.Equal(t, Replace, out.Action)
	})

	t.Run("json", func(t *testing.T) {
		b, err := json.Marshal(in)
		require.NoError(t, err)
		var out Config
		require.NoError(t, json.Unmarshal(b, &out))
		assert.Equal(t, []string{"a", "b"}, out.SourceLabels)
		assert.Equal(t, "@", out.Separator)
		assert.Equal(t, "(.+)", out.Regex.String())
		assert.Equal(t, Replace, out.Action)
	})
}

// TestConfig_explicitEmptyRoundTrip guards the marshal side of the set-tracking: an
// explicit-empty separator/replacement must survive marshal -> unmarshal instead of
// being dropped by omitempty and reverting to the defaults (";" / "$1").
func TestConfig_explicitEmptyRoundTrip(t *testing.T) {
	var in Config
	require.NoError(t, yaml.Unmarshal([]byte("source_labels: []\nseparator: \"\"\nreplacement: \"\"\naction: replace\ntarget_label: x"), &in))
	require.True(t, in.sourceLabelsSet)
	require.True(t, in.separatorSet)
	require.True(t, in.replacementSet)
	require.Empty(t, in.SourceLabels)
	require.Empty(t, in.Separator)
	require.Empty(t, in.Replacement)

	codecs := map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		"yaml": {yaml.Marshal, yaml.Unmarshal},
		"json": {json.Marshal, json.Unmarshal},
	}
	for name, codec := range codecs {
		t.Run(name, func(t *testing.T) {
			b, err := codec.marshal(in)
			require.NoError(t, err)

			var out Config
			require.NoError(t, codec.unmarshal(b, &out))
			assert.Empty(t, out.SourceLabels, "source_labels must stay explicitly empty")
			assert.True(t, out.sourceLabelsSet, "sourceLabelsSet must survive the round-trip")
			assert.Empty(t, out.Separator, "separator must stay explicitly empty")
			assert.True(t, out.separatorSet, "separatorSet must survive the round-trip")
			assert.Empty(t, out.Replacement, "replacement must stay explicitly empty")
			assert.True(t, out.replacementSet, "replacementSet must survive the round-trip")
		})
	}
}

// TestConfig_unmarshalThenNew confirms an unmarshaled rule validates and applies.
func TestConfig_unmarshalThenNew(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte(`
source_labels: [__name__]
regex: "drop_me"
action: drop
`), &cfg))

	proc, err := New([]Config{cfg})
	require.NoError(t, err)

	_, drop := proc.Apply(sample("drop_me", nil, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge))
	assert.True(t, drop.Dropped())
	_, keep := proc.Apply(sample("keep_me", nil, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge))
	assert.False(t, keep.Dropped())
}

func configWithAction(t *testing.T, a Action) Config {
	t.Helper()
	return Config{
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
		Action:      a,
		NameScheme:  defaultNameValidationScheme,
	}
}

func assertConfigEqual(t *testing.T, want, got Config) {
	t.Helper()
	assert.Equal(t, want.SourceLabels, got.SourceLabels, "SourceLabels")
	assert.Equal(t, want.Separator, got.Separator, "Separator")
	assert.Equal(t, want.Regex.String(), got.Regex.String(), "Regex")
	assert.Equal(t, want.Modulus, got.Modulus, "Modulus")
	assert.Equal(t, want.TargetLabel, got.TargetLabel, "TargetLabel")
	assert.Equal(t, want.Replacement, got.Replacement, "Replacement")
	assert.Equal(t, want.Action, got.Action, "Action")
	assert.Equal(t, want.NameScheme, got.NameScheme, "NameScheme")
	assert.Equal(t, want.sourceLabelsSet, got.sourceLabelsSet, "sourceLabelsSet")
	assert.Equal(t, want.separatorSet, got.separatorSet, "separatorSet")
	assert.Equal(t, want.replacementSet, got.replacementSet, "replacementSet")
}
