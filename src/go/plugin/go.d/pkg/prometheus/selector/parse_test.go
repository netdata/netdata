// SPDX-License-Identifier: GPL-3.0-or-later

package selector

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		input       string
		expectedSr  Selector
		expectedErr bool
	}{
		"sp op: only metric name": {
			input:      "go_memstats_alloc_bytes !go_memstats_* *",
			expectedSr: mustSPName("go_memstats_alloc_bytes !go_memstats_* *"),
		},
		"string op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"value"}`, OpEqual),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: mustString("label", "value"),
			},
		},
		"neg string op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"value"}`, OpNegEqual),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: Not(mustString("label", "value")),
			},
		},
		"regexp op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"valu.+"}`, OpRegexp),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: mustRegexp("label", "valu.+"),
			},
		},
		"neg regexp op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"valu.+"}`, OpNegRegexp),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: Not(mustRegexp("label", "valu.+")),
			},
		},
		"sp op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"valu*"}`, OpSimplePatterns),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: mustSP("label", "valu*"),
			},
		},
		"neg sp op: metric name with labels": {
			input: fmt.Sprintf(`go_memstats_*{label%s"valu*"}`, OpNegSimplePatterns),
			expectedSr: andSelector{
				lhs: mustSPName("go_memstats_*"),
				rhs: Not(mustSP("label", "valu*")),
			},
		},
		"metric name with several labels": {
			input: fmt.Sprintf(`go_memstats_*{label1%s"value1",label2%s"value2"}`, OpEqual, OpEqual),
			expectedSr: andSelector{
				lhs: andSelector{
					lhs: mustSPName("go_memstats_*"),
					rhs: mustString("label1", "value1"),
				},
				rhs: mustString("label2", "value2"),
			},
		},
		"only labels (unsugar)": {
			input: fmt.Sprintf(`{__name__%s"go_memstats_*",label1%s"value1",label2%s"value2"}`,
				OpSimplePatterns, OpEqual, OpEqual),
			expectedSr: andSelector{
				lhs: andSelector{
					lhs: mustSPName("go_memstats_*"),
					rhs: mustString("label1", "value1"),
				},
				rhs: mustString("label2", "value2"),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sr, err := Parse(test.input)

			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedSr, sr)
			}
		})
	}
}

func mustSPName(pattern string) Selector {
	return mustSP(labels.MetricName, pattern)
}

func mustString(name string, pattern string) Selector {
	return labelSelector{name: name, m: matcher.Must(matcher.NewStringMatcher(pattern, true, true))}
}

func mustRegexp(name string, pattern string) Selector {
	return labelSelector{name: name, m: matcher.Must(matcher.NewRegExpMatcher(pattern))}
}

func mustSP(name string, pattern string) Selector {
	return labelSelector{name: name, m: matcher.Must(matcher.NewSimplePatternsMatcher(pattern))}
}
