// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func TestProfileValidate_CuratedRelabelRules(t *testing.T) {
	tests := map[string]struct {
		rules       []relabel.Config
		wantErrText string
	}{
		"accepts constant __name__ replace": {
			rules: []relabel.Config{{
				Action:      relabel.Replace,
				TargetLabel: "__name__",
				Replacement: "renamed_metric",
			}},
		},
		"accepts structural label in source labels only": {
			rules: []relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: []string{"quantile"},
				TargetLabel:  "quantile_copy",
				Replacement:  "$1",
			}},
		},
		"rejects capture based __name__ replace": {
			rules: []relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: []string{"__name__"},
				TargetLabel:  "__name__",
				Replacement:  "${1}",
			}},
			wantErrText: "capture-based or dynamic __name__ replacement is not allowed",
		},
		"rejects dynamic replace target label": {
			rules: []relabel.Config{{
				Action:      relabel.Replace,
				TargetLabel: "${1}",
				Replacement: "foo",
			}},
			wantErrText: "dynamic 'target_label' is not allowed",
		},
		"rejects lowercase targeting __name__": {
			rules: []relabel.Config{{
				Action:       relabel.Lowercase,
				SourceLabels: []string{"foo"},
				TargetLabel:  "__name__",
			}},
			wantErrText: `"__name__" must not be targeted`,
		},
		"rejects replace targeting structural label": {
			rules: []relabel.Config{{
				Action:      relabel.Replace,
				TargetLabel: "quantile",
				Replacement: "0.9",
			}},
			wantErrText: `"quantile" is immutable`,
		},
		"rejects labeldrop matching structural label": {
			rules: []relabel.Config{{
				Action: relabel.LabelDrop,
				Regex:  relabel.MustNewRegexp("quantile"),
			}},
			wantErrText: "structural labels must not be targeted by labeldrop",
		},
		"rejects labelkeep": {
			rules: []relabel.Config{{
				Action: relabel.LabelKeep,
				Regex:  relabel.MustNewRegexp(".*"),
			}},
			wantErrText: "labelkeep is not allowed in curated mode",
		},
		"rejects dynamic labelmap replacement": {
			rules: []relabel.Config{{
				Action:      relabel.LabelMap,
				Regex:       relabel.MustNewRegexp("foo"),
				Replacement: "${1}",
			}},
			wantErrText: "dynamic 'replacement' is not allowed for labelmap",
		},
		"rejects labelmap creating __name__": {
			rules: []relabel.Config{{
				Action:      relabel.LabelMap,
				Regex:       relabel.MustNewRegexp("foo"),
				Replacement: "__name__",
			}},
			wantErrText: `"__name__" must not be created by labelmap`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			profile := Profile{
				Name:              "Demo",
				Match:             "demo_*",
				MetricsRelabeling: []MetricsRelabelingBlock{{Selector: "demo_metric", Rules: test.rules}},
				Template:          testProfileTemplate(),
			}

			err := profile.Validate(`profile "Demo"`)
			if test.wantErrText == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantErrText)
		})
	}
}

func testProfileTemplate() charttpl.Group {
	return charttpl.Group{
		Family:  "prometheus_curated",
		Metrics: []string{"demo_metric_total"},
		Charts: []charttpl.Chart{{
			ID:      "demo_chart",
			Title:   "Demo Metric",
			Context: "prometheus.demo.metric",
			Units:   "events",
			Dimensions: []charttpl.Dimension{{
				Selector: "demo_metric_total",
				Name:     "value",
			}},
		}},
	}
}
