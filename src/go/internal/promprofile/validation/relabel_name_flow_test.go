// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelabelNameFlowMutationAndReachability(t *testing.T) {
	flow := newTestRelabelNameFlowAnalyzer(t, relabelNameFlowBudget{})
	effect, err := flow.mutation(relabel.Config{
		Regex: relabel.MustNewRegexp(`(.+)`), Replacement: `service[*]_${1}`, Action: relabel.Replace,
	}, false)
	require.NoError(t, err)
	assert.Equal(t, `service\[\*\]_*`, effect.outputPattern)
	assert.True(t, effect.applicationDerived)

	reaches, err := flow.mutationsMayReach([]relabelNameMutation{effect}, `service\[\*\]_*`, true)
	require.NoError(t, err)
	assert.True(t, reaches)
	reaches, err = flow.mutationsMayReach([]relabelNameMutation{effect}, `!service\[\*\]_* service_*`, true)
	require.NoError(t, err)
	assert.False(t, reaches)
}

func TestRelabelNameFlowMutationAppliesRuleDefaults(t *testing.T) {
	flow := newTestRelabelNameFlowAnalyzer(t, relabelNameFlowBudget{})
	effect, err := flow.mutation(relabel.Config{}, true)
	require.NoError(t, err)
	assert.True(t, effect.reachable)
	assert.Equal(t, "*", effect.outputPattern)
}

func TestRelabelNameFlowHonorsAggregateIntersectionBudget(t *testing.T) {
	flow := newTestRelabelNameFlowAnalyzer(t, relabelNameFlowBudget{MaxScopeIntersections: 1})
	first, err := flow.mutation(relabel.Config{
		Regex: relabel.MustNewRegexp(`.*`), Replacement: "one_", Action: relabel.Replace,
	}, true)
	require.NoError(t, err)
	second, err := flow.mutation(relabel.Config{
		Regex: relabel.MustNewRegexp(`.*`), Replacement: "two_", Action: relabel.Replace,
	}, true)
	require.NoError(t, err)
	effects := []relabelNameMutation{first, second}
	_, err = flow.mutationsMayReach(effects, "three_*", false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "scope-intersection budget exceeded")
}

func TestRelabelNameFlowPreservedCaptureLabel(t *testing.T) {
	rewriteRegex := relabel.MustNewRegexp(`app_worker_(.+)_(temperature|requests_total)`)
	identityRule := relabel.Config{
		SourceLabels: []string{labels.MetricName}, Regex: rewriteRegex,
		TargetLabel: "worker", Replacement: `${1}`, Action: relabel.Replace,
	}
	rewriteRule := relabel.Config{
		SourceLabels: []string{labels.MetricName}, Regex: rewriteRegex,
		TargetLabel: labels.MetricName, Replacement: `app_${2}`, Action: relabel.Replace,
	}

	tests := map[string]struct {
		blocks []relabel.Block
		want   bool
	}{
		"preserved": {
			blocks: []relabel.Block{{Match: "app_*", MetricRelabelConfigs: []relabel.Config{identityRule, rewriteRule}}},
			want:   true,
		},
		"overwritten in same block": {
			blocks: []relabel.Block{{Match: "app_*", MetricRelabelConfigs: []relabel.Config{
				identityRule,
				{SourceLabels: []string{"instance"}, Regex: relabel.MustNewRegexp(`.*`), TargetLabel: "worker", Replacement: `$0`, Action: relabel.Replace},
				rewriteRule,
			}}},
		},
		"metric name changed between extraction and rewrite": {
			blocks: []relabel.Block{{Match: "*", MetricRelabelConfigs: []relabel.Config{
				identityRule,
				{
					SourceLabels: []string{labels.MetricName}, Regex: relabel.MustNewRegexp(`raw_temperature`),
					TargetLabel: labels.MetricName, Replacement: `app_worker_alpha_temperature`, Action: relabel.Replace,
				},
				rewriteRule,
			}}},
		},
		"dropped in reachable later block": {
			blocks: []relabel.Block{
				{Match: "app_*", MetricRelabelConfigs: []relabel.Config{identityRule, rewriteRule}},
				{Match: "app_temperature", MetricRelabelConfigs: []relabel.Config{{Regex: relabel.MustNewRegexp(`worker`), Action: relabel.LabelDrop}}},
			},
		},
		"untouched by disjoint later block": {
			blocks: []relabel.Block{
				{Match: "app_*", MetricRelabelConfigs: []relabel.Config{identityRule, rewriteRule}},
				{Match: "other_*", MetricRelabelConfigs: []relabel.Config{{Regex: relabel.MustNewRegexp(`worker`), Action: relabel.LabelDrop}}},
			},
			want: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			flow := newTestRelabelNameFlowAnalyzer(t, relabelNameFlowBudget{})
			label, preserved, err := flow.preservedCaptureLabel(
				tc.blocks, 0, len(tc.blocks[0].MetricRelabelConfigs)-1, rewriteRule,
				[]int{1}, []string{"app_requests_total", "app_temperature"},
			)
			require.NoError(t, err)
			assert.Equal(t, tc.want, preserved)
			if tc.want {
				assert.Equal(t, "worker", label)
			}
		})
	}
}

func newTestRelabelNameFlowAnalyzer(t *testing.T, budget relabelNameFlowBudget) *relabelNameFlowAnalyzer {
	t.Helper()
	relabelAnalyzer, err := relabel.NewAnalyzer(context.Background(), relabel.AnalysisBudget{})
	require.NoError(t, err)
	matcherAnalyzer, err := matcher.NewAnalyzer(context.Background(), matcher.AnalysisBudget{})
	require.NoError(t, err)
	flow, err := newRelabelNameFlowAnalyzer(context.Background(), relabelAnalyzer, matcherAnalyzer, budget)
	require.NoError(t, err)
	return flow
}
