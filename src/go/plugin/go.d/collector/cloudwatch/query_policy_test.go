// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func longDuration(value time.Duration) *confopt.LongDuration {
	v := confopt.LongDuration(value)
	return &v
}

func TestResolveSeriesPolicies_ProfileMetricAndRulePeriod(t *testing.T) {
	profile := cwprofiles.ResolvedProfile{Name: "service", Config: cwprofiles.Profile{
		Period: 300,
		Metrics: []cwprofiles.Metric{
			{ID: "fast", MetricName: "Fast", Statistics: []string{"average"}, Period: 60},
			{ID: "normal", MetricName: "Normal", Statistics: []string{"average"}},
		},
	}}
	base := compileProfileSeries(profile)
	require.Len(t, base, 2)
	assert.Equal(t, time.Minute, base[0].Policy.period)
	assert.Equal(t, 5*time.Minute, base[1].Policy.period)

	resolved, err := resolveSeriesPolicies("rules[0]", &QueryPolicyConfig{Period: longDuration(2 * time.Minute)}, nil, profile, base)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, resolved[0].Policy.period)
	assert.Equal(t, 2*time.Minute, resolved[1].Policy.period)
}

func TestResolveQueryPolicy_Precedence(t *testing.T) {
	profileDelay := longDuration(time.Hour)
	tests := map[string]struct {
		rule, defaults *QueryPolicyConfig
		profilePeriod  int
		profileDelay   *confopt.LongDuration
		want           queryPolicy
	}{
		"profile and built-in fallbacks": {
			profilePeriod: 300,
			want:          queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: defaultPublicationDelay},
		},
		"profile publication delay": {
			profilePeriod: 300, profileDelay: profileDelay,
			want: queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: time.Hour},
		},
		"rule defaults override profile field by field": {
			defaults: &QueryPolicyConfig{
				Period: longDuration(time.Hour), Lookback: longDuration(6 * time.Hour),
				PublicationDelay: longDuration(30 * time.Minute),
			},
			profilePeriod: 300, profileDelay: profileDelay,
			want: queryPolicy{period: time.Hour, lookback: 6 * time.Hour, publicationDelay: 30 * time.Minute},
		},
		"rule overrides defaults field by field": {
			rule: &QueryPolicyConfig{Period: longDuration(2 * time.Hour), PublicationDelay: longDuration(0)},
			defaults: &QueryPolicyConfig{
				Period: longDuration(time.Hour), Lookback: longDuration(6 * time.Hour),
				PublicationDelay: longDuration(30 * time.Minute),
			},
			profilePeriod: 300, profileDelay: profileDelay,
			want: queryPolicy{period: 2 * time.Hour, lookback: 6 * time.Hour, publicationDelay: 0},
		},
		"omitted lookback follows resolved period": {
			rule:          &QueryPolicyConfig{Period: longDuration(2 * time.Hour)},
			profilePeriod: 300,
			want:          queryPolicy{period: 2 * time.Hour, lookback: 2 * time.Hour, publicationDelay: defaultPublicationDelay},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveQueryPolicy("rules[0]", tc.rule, tc.defaults, tc.profilePeriod, tc.profileDelay)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveQueryPolicy_Validation(t *testing.T) {
	maxWholeSecondDuration := (time.Duration(1<<63-1) / time.Second) * time.Second
	tests := map[string]struct {
		policy  QueryPolicyConfig
		wantErr string
	}{
		"minimum period":                         {policy: QueryPolicyConfig{Period: longDuration(time.Minute)}},
		"maximum period":                         {policy: QueryPolicyConfig{Period: longDuration(24 * time.Hour)}},
		"period below minimum":                   {policy: QueryPolicyConfig{Period: longDuration(30 * time.Second)}, wantErr: "exact multiple"},
		"period not minute multiple":             {policy: QueryPolicyConfig{Period: longDuration(90 * time.Second)}, wantErr: "exact multiple"},
		"period above maximum":                   {policy: QueryPolicyConfig{Period: longDuration(24*time.Hour + time.Minute)}, wantErr: "between"},
		"lookback below period":                  {policy: QueryPolicyConfig{Period: longDuration(5 * time.Minute), Lookback: longDuration(time.Minute)}, wantErr: "at least"},
		"lookback not period multiple":           {policy: QueryPolicyConfig{Period: longDuration(5 * time.Minute), Lookback: longDuration(6 * time.Minute)}, wantErr: "exact multiple"},
		"exact bucket maximum":                   {policy: QueryPolicyConfig{Period: longDuration(time.Minute), Lookback: longDuration(1440 * time.Minute), PublicationDelay: longDuration(0)}},
		"bucket maximum exceeded":                {policy: QueryPolicyConfig{Period: longDuration(time.Minute), Lookback: longDuration(1441 * time.Minute), PublicationDelay: longDuration(0)}, wantErr: "maximum is 1440"},
		"exact reliable horizon":                 {policy: QueryPolicyConfig{Period: longDuration(time.Hour), Lookback: longDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: longDuration(24 * time.Hour)}},
		"reliable horizon exceeded":              {policy: QueryPolicyConfig{Period: longDuration(time.Hour), Lookback: longDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: longDuration(24*time.Hour + time.Second)}, wantErr: "exceeds 336h0m0s"},
		"duration limit cannot overflow horizon": {policy: QueryPolicyConfig{PublicationDelay: longDuration(maxWholeSecondDuration)}, wantErr: "query horizon"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveQueryPolicy("rules[0]", &tc.policy, nil, 300, nil)
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Positive(t, got.bucketCount())
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidateQueryPolicyConfig_RawDurations(t *testing.T) {
	tests := map[string]struct {
		policy  *QueryPolicyConfig
		wantErr string
	}{
		"omitted":             {},
		"explicit zero delay": {policy: &QueryPolicyConfig{PublicationDelay: longDuration(0)}},
		"zero lookback":       {policy: &QueryPolicyConfig{Lookback: longDuration(0)}, wantErr: "must be positive"},
		"negative delay":      {policy: &QueryPolicyConfig{PublicationDelay: longDuration(-time.Second)}, wantErr: "must not be negative"},
		"subsecond lookback":  {policy: &QueryPolicyConfig{Lookback: longDuration(time.Second + time.Millisecond)}, wantErr: "whole seconds"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateQueryPolicyConfig("rules[0].query", tc.policy)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestQueryPolicy_RecentlyActiveUsesFullHorizon(t *testing.T) {
	assert.True(t, (queryPolicy{period: time.Hour, lookback: time.Hour, publicationDelay: time.Hour}).useRecentlyActive())
	assert.False(t, (queryPolicy{period: time.Hour, lookback: time.Hour, publicationDelay: time.Hour + time.Second}).useRecentlyActive())
}
