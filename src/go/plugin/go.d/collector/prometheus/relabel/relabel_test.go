// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"testing"

	"github.com/grafana/regexp"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

func TestRegexp_String(t *testing.T) {
	tests := map[string]struct {
		re   Regexp
		want string
	}{
		"NewRegexp returns the un-anchored source":  {re: MustNewRegexp("(.*)_total"), want: "(.*)_total"},
		"NewRegexp with empty source":               {re: MustNewRegexp(""), want: ""},
		"zero value":                                {re: Regexp{}, want: ""},
		"raw non-anchored value is safe (no panic)": {re: Regexp{Regexp: regexp.MustCompile("x")}, want: ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got string
			require.NotPanics(t, func() { got = tc.re.String() })
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProcessor_Apply(t *testing.T) {
	tests := map[string]struct {
		cfgs             []Config
		in               prompkg.Sample
		want             prompkg.Sample
		keep             bool
		sameLabelBacking bool
	}{
		"labelmap skips __name__ and does not re-map its own output": {
			cfgs: []Config{{
				Regex:       MustNewRegexp("(.+)"),
				Replacement: "copy_${1}",
				Action:      LabelMap,
			}},
			in: sample("m", map[string]string{"job": "api"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("m", map[string]string{
				"job":      "api",
				"copy_job": "api",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"explicit empty separator joins without the default ';'": {
			cfgs: []Config{{
				SourceLabels: []string{"a", "b"},
				Separator:    "",
				separatorSet: true,
				TargetLabel:  "c",
				Action:       Replace,
			}},
			in:   sample("m", map[string]string{"a": "foo", "b": "bar"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("m", map[string]string{"a": "foo", "b": "bar", "c": "foobar"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"non-canonical action is canonicalized (KEEP -> keep)": {
			cfgs: []Config{{
				SourceLabels: []string{"a"},
				Regex:        MustNewRegexp("foo"),
				Action:       Action("KEEP"),
			}},
			in:   sample("m", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("m", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"zero rules pass through without rematerializing labels": {
			in:               sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want:             sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in: sample("test_total", map[string]string{"a": "foo"}, 1.5, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("test_total", map[string]string{
				"a": "foo",
				"b": "choo",
			}, 1.5, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			in:   sample("http_requests_total", map[string]string{"method": "GET"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("http_requests", map[string]string{"method": "GET"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			in: sample("http.requests_total", map[string]string{"method": "get"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("http.requests_total", map[string]string{
				"method":       "get",
				"method_upper": "GET",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			in:               sample("http_requests_total", map[string]string{"method": "GET"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
			want:             sample("http_requests.total", map[string]string{"method": "GET"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			}, 42, prompkg.SampleKindHistogramBucket, commonmodel.MetricTypeHistogram),
			want: sample("nginx_request_duration_seconds_bucket", map[string]string{
				"le":     "0.5",
				"method": "GET",
			}, 42, prompkg.SampleKindHistogramBucket, commonmodel.MetricTypeHistogram),
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
			}, 7, prompkg.SampleKindSummaryQuantile, commonmodel.MetricTypeSummary),
			want: sample("nginx_request_duration_seconds", map[string]string{
				"method":   "GET",
				"quantile": "0.9",
			}, 7, prompkg.SampleKindSummaryQuantile, commonmodel.MetricTypeSummary),
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
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in:               sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want:             sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
			want: sample("request.total", map[string]string{
				"rename_to": "request.total",
				"seen_name": "prefix_request.total",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			in: sample("test_metric", map[string]string{"__tmp_port": "1234", "__port1": "1234"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in:   sample("test_metric", map[string]string{"__tmp_port": "1234", "__port1": "1234"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in: sample("test_metric", map[string]string{"a": "foo", "b": "bar", "c": "baz"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in: sample("test_metric", map[string]string{"a": "foo", "b1": "bar", "b2": "baz"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labeldrop removes matching labels": {
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelDrop,
				},
			},
			in: sample("test_metric", map[string]string{"a": "foo", "b1": "bar", "b2": "baz"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labeldrop does not drop __name__": {
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("(.+)"),
					Action: LabelDrop,
				},
			},
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
		},
		"labelkeep keeps __name__ even when the regex excludes it": {
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelKeep,
				},
			},
			in:   sample("test_metric", map[string]string{"b1": "bar", "other": "x"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{"b1": "bar"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: true,
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
			in: sample("test_metric", map[string]string{"foo": "bAr123Foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"foo":           "bAr123Foo",
				"foo_lowercase": "bar123foo",
				"foo_uppercase": "BAR123FOO",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in: sample("test_metric", map[string]string{"a": "foo", "b": "bar"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			want: sample("test_metric", map[string]string{
				"a": "foo",
			}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
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
			in:   sample("test_metric", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			keep: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := New(test.cfgs)
			require.NoError(t, err)

			got, drop := p.Apply(test.in)
			require.Equal(t, test.keep, !drop.Dropped())
			if !drop.Dropped() {
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
		"rejects labeldrop with explicit empty source_labels": {
			cfgs: []Config{{
				Action:          LabelDrop,
				Regex:           MustNewRegexp("foo"),
				sourceLabelsSet: true,
			}},
			wantErrText: `requires only 'regex'`,
		},
		"rejects replace with invalid regex-var target label under legacy": {
			cfgs: []Config{{
				Action:       Replace,
				SourceLabels: []string{"a"},
				TargetLabel:  "${1}.x",
				NameScheme:   commonmodel.LegacyValidation,
			}},
			wantErrText: `invalid 'target_label'`,
		},
		"rejects labelmap with invalid replacement under legacy": {
			cfgs: []Config{{
				Action:      LabelMap,
				Regex:       MustNewRegexp("(.+)"),
				Replacement: "bad.name",
				NameScheme:  commonmodel.LegacyValidation,
			}},
			wantErrText: `invalid 'replacement'`,
		},
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
		"defaults to utf8 validation when unset (accepts a target legacy would reject)": {
			cfgs: []Config{{
				Action:      Lowercase,
				TargetLabel: "${3}",
			}},
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

func sample(name string, lbs map[string]string, value float64, kind prompkg.SampleKind, familyType commonmodel.MetricType) prompkg.Sample {
	return prompkg.Sample{
		Name:       name,
		Labels:     labels.FromMap(lbs),
		Value:      value,
		Kind:       kind,
		FamilyType: familyType,
	}
}

func TestProcessor_Apply_dropInfo(t *testing.T) {
	tests := map[string]struct {
		cfgs       []Config
		in         prompkg.Sample
		wantReason DropReason
		wantRule   int
		wantAction Action
	}{
		"drop rule matched": {
			cfgs:       []Config{{SourceLabels: []string{"a"}, Regex: MustNewRegexp("foo"), Action: Drop}},
			in:         sample("m", map[string]string{"a": "foo"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			wantReason: DropReasonDropRuleMatched,
			wantRule:   0,
			wantAction: Drop,
		},
		"dropequal matched": {
			cfgs:       []Config{{SourceLabels: []string{"a"}, TargetLabel: "b", Action: DropEqual}},
			in:         sample("m", map[string]string{"a": "x", "b": "x"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			wantReason: DropReasonDropEqualMatched,
			wantRule:   0,
			wantAction: DropEqual,
		},
		"keepequal did not match": {
			cfgs:       []Config{{SourceLabels: []string{"a"}, TargetLabel: "b", Action: KeepEqual}},
			in:         sample("m", map[string]string{"a": "x", "b": "y"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			wantReason: DropReasonKeepEqualMismatch,
			wantRule:   0,
			wantAction: KeepEqual,
		},
		"keep rule did not match": {
			cfgs:       []Config{{SourceLabels: []string{"a"}, Regex: MustNewRegexp("foo"), Action: Keep}},
			in:         sample("m", map[string]string{"a": "bar"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			wantReason: DropReasonKeepRuleMismatch,
			wantRule:   0,
			wantAction: Keep,
		},
		"invalid metric name (replace empties __name__)": {
			cfgs: []Config{{
				SourceLabels:   []string{commonmodel.MetricNameLabel},
				Regex:          MustNewRegexp("(.*)"),
				TargetLabel:    commonmodel.MetricNameLabel,
				Replacement:    "",
				replacementSet: true,
				Action:         Replace,
			}},
			in:         sample("m", map[string]string{"a": "x"}, 1, prompkg.SampleKindScalar, commonmodel.MetricTypeGauge),
			wantReason: DropReasonInvalidMetricName,
			wantRule:   -1,
			wantAction: "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := New(test.cfgs)
			require.NoError(t, err)

			got, drop := p.Apply(test.in)
			require.True(t, drop.Dropped())
			assert.Equal(t, test.wantReason, drop.Reason)
			assert.Equal(t, test.wantRule, drop.RuleIndex)
			assert.Equal(t, test.wantAction, drop.Action)
			// Apply returns the original sample on drop so the caller can log it.
			assert.Equal(t, test.in, got)
		})
	}
}
