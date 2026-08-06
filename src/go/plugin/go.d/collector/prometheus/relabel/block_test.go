// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

func TestNewPipeline_ValidatesBlocks(t *testing.T) {
	tests := map[string]struct {
		blocks  []Block
		wantErr string
	}{
		"no blocks": {},
		"valid": {
			blocks: []Block{{
				Match:                "app_*",
				MetricRelabelConfigs: []Config{{SourceLabels: []string{"__name__"}, Regex: MustNewRegexp("x"), Action: Drop}},
			}},
		},
		"missing match": {
			blocks:  []Block{{MetricRelabelConfigs: []Config{{Action: Drop}}}},
			wantErr: "relabeling[0]: 'match' is required",
		},
		"invalid match": {
			blocks: []Block{{
				Match:                "[a-",
				MetricRelabelConfigs: []Config{{Action: Drop}},
			}},
			wantErr: "relabeling[0]: invalid 'match'",
		},
		"missing rules": {
			blocks:  []Block{{Match: "app_*"}},
			wantErr: "relabeling[0]: 'metric_relabel_configs' is empty",
		},
		"invalid rule": {
			blocks: []Block{{
				Match:                "app_*",
				MetricRelabelConfigs: []Config{{Action: "bogus"}},
			}},
			wantErr: "relabeling[0]: rule 0:",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pipeline, err := NewPipeline(tc.blocks)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, pipeline)
				return
			}
			require.NoError(t, err)
			if len(tc.blocks) == 0 {
				assert.Nil(t, pipeline)
			} else {
				assert.NotNil(t, pipeline)
			}
		})
	}
}

func TestPipeline_ApplyUsesCurrentNameInBlockOrder(t *testing.T) {
	pipeline, err := NewPipeline([]Block{
		{
			Match: "app_*",
			MetricRelabelConfigs: []Config{{
				SourceLabels: []string{commonmodel.MetricNameLabel},
				Regex:        MustNewRegexp("app_(.*)"),
				TargetLabel:  commonmodel.MetricNameLabel,
				Replacement:  "renamed_${1}",
				Action:       Replace,
			}},
		},
		{
			Match: "renamed_*",
			MetricRelabelConfigs: []Config{{
				SourceLabels: []string{commonmodel.MetricNameLabel},
				TargetLabel:  "seen_name",
				Action:       Replace,
			}},
		},
	})
	require.NoError(t, err)

	in := sample("app_requests_total", map[string]string{"method": "GET"}, 7, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter)
	want := sample("renamed_requests_total", map[string]string{
		"method":    "GET",
		"seen_name": "renamed_requests_total",
	}, 7, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter)

	got, drop := pipeline.Apply(in)
	require.False(t, drop.Dropped())
	assert.Equal(t, want, got)
}

func TestPipeline_Matches(t *testing.T) {
	pipeline, err := NewPipeline([]Block{
		{Match: "app_*", MetricRelabelConfigs: []Config{{Action: Drop}}},
		{Match: "db_*", MetricRelabelConfigs: []Config{{Action: Drop}}},
	})
	require.NoError(t, err)

	assert.True(t, pipeline.Matches("app_requests_total"))
	assert.True(t, pipeline.Matches("db_queries_total"))
	assert.False(t, pipeline.Matches("other_metric"))
}

func TestCloneBlocks_IsIndependent(t *testing.T) {
	original := []Block{{
		Match: "app_*",
		MetricRelabelConfigs: []Config{{
			SourceLabels: []string{"method"},
			Regex:        MustNewRegexp("(.+)"),
			TargetLabel:  "verb",
			Action:       Replace,
		}},
	}}

	cloned := CloneBlocks(original)
	require.NotSame(t, original[0].MetricRelabelConfigs[0].Regex.Regexp, cloned[0].MetricRelabelConfigs[0].Regex.Regexp)
	cloned[0].Match = "changed_*"
	cloned[0].MetricRelabelConfigs[0].SourceLabels[0] = "changed"
	cloned[0].MetricRelabelConfigs[0].Regex.Regexp.Longest()

	assert.Equal(t, "app_*", original[0].Match)
	assert.Equal(t, []string{"method"}, original[0].MetricRelabelConfigs[0].SourceLabels)

	empty := CloneBlocks([]Block{{MetricRelabelConfigs: []Config{{SourceLabels: []string{}}}}})
	require.NotNil(t, empty)
	require.NotNil(t, empty[0].MetricRelabelConfigs)
	require.NotNil(t, empty[0].MetricRelabelConfigs[0].SourceLabels)
	assert.Nil(t, CloneBlocks(nil))
}

func TestNewPipeline_OwnsRuleConfiguration(t *testing.T) {
	blocks := []Block{{
		Match: "app_*",
		MetricRelabelConfigs: []Config{{
			SourceLabels: []string{"method"},
			Regex:        MustNewRegexp("(.+)"),
			TargetLabel:  "verb",
			Action:       Replace,
		}},
	}}

	pipeline, err := NewPipeline(blocks)
	require.NoError(t, err)
	require.NotSame(t, blocks[0].MetricRelabelConfigs[0].Regex.Regexp, pipeline.blocks[0].proc.cfgs[0].Regex.Regexp)

	blocks[0].MetricRelabelConfigs[0].SourceLabels[0] = "changed"
	in := sample("app_requests_total", map[string]string{"method": "GET"}, 7, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter)
	got, drop := pipeline.Apply(in)
	require.False(t, drop.Dropped())
	assert.Equal(t, "GET", got.Labels.Get("verb"))
}
