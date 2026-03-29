// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"encoding/json"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

func TestProcessor_Apply(t *testing.T) {
	tests := map[string]struct {
		cfgs             []Config
		in               promscrapemodel.Sample
		want             promscrapemodel.Sample
		keep             bool
		sameLabelBacking bool
	}{
		"zero rules pass through without rematerializing labels": {
			in:               sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want:             sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep:             true,
			sameLabelBacking: true,
		},
		"replace rewrites label and preserves sample payload": {
			cfgs: []Config{
				{
					SourceLabels: []string{"a"},
					Regex:        MustNewRegexp("f(.*)"),
					TargetLabel:  "b",
					Replacement:  "ch${1}",
					Action:       Replace,
				},
			},
			in: sample("test_total", map[string]string{"a": "foo"}, 1.5, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("test_total", map[string]string{
				"a": "foo",
				"b": "choo",
			}, 1.5, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			keep: true,
		},
		"replace can rewrite __name__": {
			cfgs: []Config{
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					Regex:        MustNewRegexp("(.*)_total"),
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "${1}",
					Action:       Replace,
				},
			},
			in:   sample("http_requests_total", map[string]string{"method": "GET"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("http_requests", map[string]string{"method": "GET"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			keep: true,
		},
		"label-only relabel preserves existing utf8 metric name": {
			cfgs: []Config{
				{
					SourceLabels: []string{"method"},
					TargetLabel:  "method_upper",
					Action:       Uppercase,
				},
			},
			in: sample("http.requests_total", map[string]string{"method": "get"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("http.requests_total", map[string]string{
				"method":       "get",
				"method_upper": "GET",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			keep: true,
		},
		"replace rewrites __name__ with explicit utf8 scheme at runtime": {
			cfgs: []Config{
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					Regex:        MustNewRegexp("(.*)_total"),
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "${1}.total",
					Action:       Replace,
					NameScheme:   commonmodel.UTF8Validation,
				},
			},
			in:               sample("http_requests_total", map[string]string{"method": "GET"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			want:             sample("http_requests.total", map[string]string{"method": "GET"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			keep:             true,
			sameLabelBacking: true,
		},
		"histogram bucket relabel preserves kind and family type": {
			cfgs: []Config{
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					Regex:        MustNewRegexp("(.*)"),
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "nginx_${1}",
					Action:       Replace,
				},
			},
			in: sample("request_duration_seconds_bucket", map[string]string{
				"le":     "0.5",
				"method": "GET",
			}, 42, promscrapemodel.SampleKindHistogramBucket, commonmodel.MetricTypeHistogram),
			want: sample("nginx_request_duration_seconds_bucket", map[string]string{
				"le":     "0.5",
				"method": "GET",
			}, 42, promscrapemodel.SampleKindHistogramBucket, commonmodel.MetricTypeHistogram),
			keep:             true,
			sameLabelBacking: true,
		},
		"summary quantile relabel preserves kind and family type": {
			cfgs: []Config{
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					Regex:        MustNewRegexp("(.*)"),
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "nginx_${1}",
					Action:       Replace,
				},
			},
			in: sample("request_duration_seconds", map[string]string{
				"method":   "GET",
				"quantile": "0.9",
			}, 7, promscrapemodel.SampleKindSummaryQuantile, commonmodel.MetricTypeSummary),
			want: sample("nginx_request_duration_seconds", map[string]string{
				"method":   "GET",
				"quantile": "0.9",
			}, 7, promscrapemodel.SampleKindSummaryQuantile, commonmodel.MetricTypeSummary),
			keep:             true,
			sameLabelBacking: true,
		},
		"drop drops matching input": {
			cfgs: []Config{
				{
					SourceLabels: []string{"a"},
					Regex:        MustNewRegexp(".*o.*"),
					Action:       Drop,
				},
			},
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
		"drop uses anchored regex semantics": {
			cfgs: []Config{
				{
					SourceLabels: []string{"a"},
					Regex:        MustNewRegexp("f|o"),
					Action:       Drop,
				},
			},
			in:               sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want:             sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep:             true,
			sameLabelBacking: true,
		},
		"keep drops non matching input": {
			cfgs: []Config{
				{
					SourceLabels: []string{"a"},
					Regex:        MustNewRegexp("no-match"),
					Action:       Keep,
				},
			},
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
		"ordered rules see prior __name__ rewrite": {
			cfgs: []Config{
				{
					SourceLabels: []string{"rename_to"},
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "$1",
					Action:       Replace,
					NameScheme:   commonmodel.UTF8Validation,
				},
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					TargetLabel:  "seen_name",
					Replacement:  "prefix_$1",
					Action:       Replace,
				},
			},
			in: sample("request_total", map[string]string{
				"rename_to": "request.total",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("request.total", map[string]string{
				"rename_to": "request.total",
				"seen_name": "prefix_request.total",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
			keep: true,
		},
		"keepequal keeps when values match": {
			cfgs: []Config{
				{
					SourceLabels: []string{"__tmp_port"},
					TargetLabel:  "__port1",
					Action:       KeepEqual,
				},
			},
			in: sample("test_metric", map[string]string{"__tmp_port": "1234", "__port1": "1234"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"dropequal drops when values match": {
			cfgs: []Config{
				{
					SourceLabels: []string{"__tmp_port"},
					TargetLabel:  "__port1",
					Action:       DropEqual,
				},
			},
			in:   sample("test_metric", map[string]string{"__tmp_port": "1234", "__port1": "1234"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
		"hashmod matches upstream example": {
			cfgs: []Config{
				{
					SourceLabels: []string{"c"},
					TargetLabel:  "d",
					Action:       HashMod,
					Modulus:      1000,
				},
			},
			in: sample("test_metric", map[string]string{"a": "foo", "b": "bar", "c": "baz"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labelmap copies matching labels": {
			cfgs: []Config{
				{
					Regex:       MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      LabelMap,
				},
			},
			in: sample("test_metric", map[string]string{"a": "foo", "b1": "bar", "b2": "baz"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labeldrop removes matching labels": {
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelDrop,
				},
			},
			in: sample("test_metric", map[string]string{"a": "foo", "b1": "bar", "b2": "baz"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labelkeep can drop __name__ and therefore drop the sample": {
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelKeep,
				},
			},
			in:   sample("test_metric", map[string]string{"b1": "bar"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
		"lowercase and uppercase write derived labels": {
			cfgs: []Config{
				{
					SourceLabels: []string{"foo"},
					TargetLabel:  "foo_uppercase",
					Action:       Uppercase,
				},
				{
					SourceLabels: []string{"foo"},
					TargetLabel:  "foo_lowercase",
					Action:       Lowercase,
				},
			},
			in: sample("test_metric", map[string]string{"foo": "bAr123Foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"foo":           "bAr123Foo",
				"foo_lowercase": "bar123foo",
				"foo_uppercase": "BAR123FOO",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"explicit empty replacement deletes target label": {
			cfgs: []Config{
				{
					SourceLabels:   []string{"a"},
					TargetLabel:    "b",
					Replacement:    "",
					Action:         Replace,
					replacementSet: true,
				},
			},
			in: sample("test_metric", map[string]string{"a": "foo", "b": "bar"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
			}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"invalid final metric name drops the sample": {
			cfgs: []Config{
				{
					SourceLabels:   []string{commonmodel.MetricNameLabel},
					TargetLabel:    commonmodel.MetricNameLabel,
					Replacement:    "",
					Action:         Replace,
					replacementSet: true,
				},
			},
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := New(test.cfgs)
			require.NoError(t, err)

			got, keep := p.Apply(test.in)
			require.Equal(t, test.keep, keep)
			if keep {
				assert.Equal(t, test.want, got)
				if test.sameLabelBacking && len(test.in.Labels) > 0 {
					require.NotEmpty(t, got.Labels)
					assert.True(t, &got.Labels[0] == &test.in.Labels[0])
				}
			}
		})
	}
}

func TestNew_Validate(t *testing.T) {
	tests := map[string]struct {
		cfgs        []Config
		wantErrText string
	}{
		"rejects unknown action": {
			cfgs:        []Config{{Action: Action("wat")}},
			wantErrText: `unknown relabel action "wat"`,
		},
		"rejects missing target label for replace": {
			cfgs:        []Config{{Action: Replace}},
			wantErrText: `requires 'target_label' value`,
		},
		"rejects hashmod without modulus": {
			cfgs: []Config{{
				Action:      HashMod,
				TargetLabel: "d",
			}},
			wantErrText: `requires non-zero modulus`,
		},
		"rejects labeldrop extra fields": {
			cfgs: []Config{{
				Action:         LabelDrop,
				Regex:          MustNewRegexp("foo"),
				Replacement:    "bar",
				replacementSet: true,
			}},
			wantErrText: `requires only 'regex'`,
		},
		"rejects keepequal with replacement": {
			cfgs: []Config{{
				Action:         KeepEqual,
				TargetLabel:    "__port1",
				Replacement:    "bar",
				replacementSet: true,
			}},
			wantErrText: `'replacement' can not be set for keepequal action`,
		},
		"rejects legacy invalid target label": {
			cfgs: []Config{{
				Action:      Lowercase,
				TargetLabel: "${3}",
				NameScheme:  commonmodel.LegacyValidation,
			}},
			wantErrText: `"${3}" is invalid 'target_label' for lowercase action`,
		},
		"accepts utf8 target label for lowercase": {
			cfgs: []Config{{
				Action:      Lowercase,
				TargetLabel: "${3}",
				NameScheme:  commonmodel.UTF8Validation,
			}},
		},
		"defaults to legacy validation when unset": {
			cfgs: []Config{{
				Action:      Lowercase,
				TargetLabel: "${3}",
			}},
			wantErrText: `"${3}" is invalid 'target_label' for lowercase action`,
		},
		"rejects invalid name scheme": {
			cfgs: []Config{{
				Action:      Lowercase,
				TargetLabel: "foo",
				NameScheme:  commonmodel.ValidationScheme(99),
			}},
			wantErrText: `unknown relabel config name validation method specified`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := New(test.cfgs)
			if test.wantErrText == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, test.wantErrText)
		})
	}
}

func TestConfig_UnmarshalYAML_PreservesExplicitEmptyReplacement(t *testing.T) {
	tests := map[string]struct {
		unmarshal func([]byte, any) error
		payload   string
	}{
		"yaml": {
			unmarshal: yaml.Unmarshal,
			payload: `
source_labels: [a]
target_label: b
replacement: ""
action: replace
`,
		},
		"json": {
			unmarshal: json.Unmarshal,
			payload:   `{"source_labels":["a"],"target_label":"b","replacement":"","action":"replace"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			err := test.unmarshal([]byte(test.payload), &cfg)
			require.NoError(t, err)
			assert.Equal(t, "", cfg.Replacement)
			assert.True(t, cfg.replacementSet)

			p, err := New([]Config{cfg})
			require.NoError(t, err)

			got, keep := p.Apply(sample("test_metric", map[string]string{"a": "foo", "b": "bar"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge))
			require.True(t, keep)
			assert.Equal(t, sample("test_metric", map[string]string{"a": "foo"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge), got)
		})
	}
}

func TestConfig_Unmarshal_PreservesExplicitEmptySeparator(t *testing.T) {
	tests := map[string]struct {
		unmarshal func([]byte, any) error
		payload   string
	}{
		"yaml": {
			unmarshal: yaml.Unmarshal,
			payload: `
source_labels: [a, b]
separator: ""
target_label: c
action: replace
`,
		},
		"json": {
			unmarshal: json.Unmarshal,
			payload:   `{"source_labels":["a","b"],"separator":"","target_label":"c","action":"replace"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			err := test.unmarshal([]byte(test.payload), &cfg)
			require.NoError(t, err)
			assert.Equal(t, "", cfg.Separator)
			assert.True(t, cfg.separatorSet)

			p, err := New([]Config{cfg})
			require.NoError(t, err)

			got, keep := p.Apply(sample("test_metric", map[string]string{"a": "foo", "b": "bar"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge))
			require.True(t, keep)
			assert.Equal(t, sample("test_metric", map[string]string{"a": "foo", "b": "bar", "c": "foobar"}, 1, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeGauge), got)
		})
	}
}

func TestConfig_Unmarshal_RejectsExplicitEmptySourceLabelsForLabelFilters(t *testing.T) {
	tests := map[string]struct {
		unmarshal func([]byte, any) error
		payload   string
	}{
		"yaml labeldrop": {
			unmarshal: yaml.Unmarshal,
			payload: `
source_labels: []
regex: foo
action: labeldrop
`,
		},
		"json labeldrop": {
			unmarshal: json.Unmarshal,
			payload:   `{"source_labels":[],"regex":"foo","action":"labeldrop"}`,
		},
		"yaml labelkeep": {
			unmarshal: yaml.Unmarshal,
			payload: `
source_labels: []
regex: foo
action: labelkeep
`,
		},
		"json labelkeep": {
			unmarshal: json.Unmarshal,
			payload:   `{"source_labels":[],"regex":"foo","action":"labelkeep"}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			err := test.unmarshal([]byte(test.payload), &cfg)
			require.NoError(t, err)
			assert.True(t, cfg.sourceLabelsSet)
			assert.Empty(t, cfg.SourceLabels)

			_, err = New([]Config{cfg})
			require.ErrorContains(t, err, "requires only 'regex'")
		})
	}
}

func sample(name string, lbs map[string]string, value float64, kind promscrapemodel.SampleKind, familyType commonmodel.MetricType) promscrapemodel.Sample {
	return promscrapemodel.Sample{
		Name:       name,
		Labels:     labels.FromMap(lbs),
		Value:      value,
		Kind:       kind,
		FamilyType: familyType,
	}
}
