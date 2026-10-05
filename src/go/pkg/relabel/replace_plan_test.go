// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"fmt"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplacePlanMatchesRegexpEvaluation checks every replace shortcut against the
// general path, which evaluates the regex and expands both templates with the
// regexp engine. It walks every combination of regex, target template,
// replacement template, name scheme and source value below.
func TestReplacePlanMatchesRegexpEvaluation(t *testing.T) {
	regexes := []*Regexp{nil, new(MustNewRegexp("(.*)")), new(MustNewRegexp("(.+)")), new(MustNewRegexp("a(.*)")),
		new(MustNewRegexp("(?P<name>.*)")), new(MustNewRegexp("(.*)(.*)")), new(MustNewRegexp(".*"))}
	targets := []string{"dst", "src", commonmodel.MetricNameLabel, "$1", "${1}", "x_$1", "${name}", "t_${2}"}
	replacements := []string{"$1", "${1}", "$2", "${name}", "x${1}y", "$01", "$$1", "", "static", "$1$1"}
	schemes := []commonmodel.ValidationScheme{commonmodel.LegacyValidation, commonmodel.UTF8Validation}
	values := []string{"", "a", "abc", "a\nb", "ümlaut", "$1", "dst", "x y", "9lead", "value_1"}

	var cases, shortcut int
	for _, re := range regexes {
		for _, target := range targets {
			for _, replacement := range replacements {
				for _, scheme := range schemes {
					cfg := Config{
						SourceLabels:   []string{"src"},
						TargetLabel:    target,
						Replacement:    replacement,
						replacementSet: true,
						Action:         Replace,
						NameScheme:     scheme,
					}
					if re != nil {
						cfg.Regex = *re
					}
					planned, err := New([]Config{cfg})
					if err != nil {
						continue
					}
					general, err := New([]Config{cfg})
					require.NoError(t, err)
					general.replacePlans[0] = replacePlan{}
					if planned.replacePlans[0] != (replacePlan{}) {
						shortcut++
					}

					for _, value := range values {
						cases++
						record := Record{
							Name:   "metric",
							Labels: labels.FromStrings("dst", "old", "src", value),
						}
						wantRecord, wantDrop := general.Apply(record)
						gotRecord, gotDrop := planned.Apply(record)
						msg := fmt.Sprintf("regex=%v target=%q replacement=%q value=%q", re, target, replacement, value)
						assert.Equal(t, wantDrop, gotDrop, msg)
						assert.Equal(t, wantRecord, gotRecord, msg)
					}
				}
			}
		}
	}
	require.NotZero(t, shortcut, "no configuration took a shortcut")
	t.Logf("%d records over %d shortcut configurations", cases, shortcut)
}

func TestNewReplacePlan(t *testing.T) {
	tests := map[string]struct {
		cfg  Config
		want replacePlan
	}{
		"default regex and replacement": {
			cfg: Config{
				SourceLabels: []string{"src"},
				TargetLabel:  "dst",
			},
			want: replacePlan{
				constantTarget: true,
				matchAll:       true,
				identity:       true,
			},
		},
		"explicit default regex and braced capture": {
			cfg: Config{
				SourceLabels: []string{"src"},
				Regex:        MustNewRegexp("(.*)"),
				TargetLabel:  "dst",
				Replacement:  "${1}",
			},
			want: replacePlan{
				constantTarget: true,
				matchAll:       true,
				identity:       true,
			},
		},
		"default regex with a composed replacement": {
			cfg: Config{
				SourceLabels: []string{"src"},
				TargetLabel:  "dst",
				Replacement:  "v_$1",
			},
			want: replacePlan{
				constantTarget: true,
				matchAll:       true,
			},
		},
		"custom regex and templated target": {
			cfg: Config{
				SourceLabels: []string{"src"},
				Regex:        MustNewRegexp("(.+)"),
				TargetLabel:  "${1}",
			},
			want: replacePlan{},
		},
		"other actions have no plan": {
			cfg: Config{
				SourceLabels: []string{"src"},
				TargetLabel:  "dst",
				Action:       Lowercase,
			},
			want: replacePlan{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := New([]Config{test.cfg})
			require.NoError(t, err)
			assert.Equal(t, []replacePlan{test.want}, p.replacePlans)
		})
	}
}
