// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func longDuration(value time.Duration) *confopt.LongDuration {
	v := confopt.LongDuration(value)
	return &v
}

func profileQuery(period time.Duration) cwquery.Config {
	return cwquery.Config{Period: longDuration(period)}
}

func TestQueryWindow(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)

	tests := map[string]struct {
		policy             queryPolicy
		wantStart, wantEnd int64
	}{
		"5m period, 10m delay": {policy: queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: 10 * time.Minute}, wantStart: 999_999_000, wantEnd: 999_999_300},
		"5m period, 2m delay":  {policy: queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: 2 * time.Minute}, wantStart: 999_999_300, wantEnd: 999_999_600},
		"daily settled bucket": {policy: queryPolicy{period: 24 * time.Hour, lookback: 24 * time.Hour, publicationDelay: 24 * time.Hour}, wantStart: 999_820_800, wantEnd: 999_907_200},
		"15m rolling lookback": {policy: queryPolicy{period: 5 * time.Minute, lookback: 15 * time.Minute, publicationDelay: 10 * time.Minute}, wantStart: 999_998_400, wantEnd: 999_999_300},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			start, end := queryWindow(now, tc.policy)
			assert.Equal(t, tc.wantStart, start.Unix(), "start")
			assert.Equal(t, tc.wantEnd, end.Unix(), "end")
			assert.Equal(t, int64(tc.policy.lookback/time.Second), end.Unix()-start.Unix(), "window length == lookback")
			assert.Zero(t, end.Unix()%int64(tc.policy.period/time.Second), "end is aligned to a period boundary")
			assert.True(t, end.Before(now), "window ends in the past")
		})
	}
}

func TestResolveSeriesPolicies_ProfileMetricAndRulePeriod(t *testing.T) {
	profile := cwprofiles.ResolvedProfile{Name: "service", Config: cwprofiles.Profile{
		Query: profileQuery(5 * time.Minute),
		Metrics: []cwprofiles.Metric{
			{ID: "fast", MetricName: "Fast", Statistics: []string{"average"}, Query: &cwquery.Config{Period: longDuration(time.Minute)}},
			{ID: "normal", MetricName: "Normal", Statistics: []string{"average"}},
		},
	}}
	base := compileProfileSeries(profile)
	require.Len(t, base, 2)

	resolved, err := resolveSeriesPolicies("rules[0]", nil, nil, profile, base)
	require.NoError(t, err)
	assert.Equal(t, time.Minute, resolved[0].Policy.period)
	assert.Equal(t, 5*time.Minute, resolved[1].Policy.period)

	resolved, err = resolveSeriesPolicies("rules[0]", &cwquery.Config{Period: longDuration(2 * time.Minute)}, nil, profile, base)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, resolved[0].Policy.period)
	assert.Equal(t, 2*time.Minute, resolved[1].Policy.period)
}

func TestResolveQueryPolicy_Precedence(t *testing.T) {
	tests := map[string]struct {
		rule, defaults *cwquery.Config
		metric         *cwquery.Config
		profile        cwquery.Config
		want           queryPolicy
	}{
		"profile and built-in fallbacks": {
			profile: profileQuery(5 * time.Minute),
			want:    queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: defaultPublicationDelay},
		},
		"profile publication delay": {
			profile: cwquery.Config{Period: longDuration(5 * time.Minute), PublicationDelay: longDuration(time.Hour)},
			want:    queryPolicy{period: 5 * time.Minute, lookback: 5 * time.Minute, publicationDelay: time.Hour},
		},
		"metric overrides profile field by field": {
			metric:  &cwquery.Config{Period: longDuration(time.Hour), Lookback: longDuration(6 * time.Hour), PublicationDelay: longDuration(30 * time.Minute)},
			profile: cwquery.Config{Period: longDuration(5 * time.Minute), Lookback: longDuration(10 * time.Minute), PublicationDelay: longDuration(time.Hour)},
			want:    queryPolicy{period: time.Hour, lookback: 6 * time.Hour, publicationDelay: 30 * time.Minute},
		},
		"rule defaults override profile field by field": {
			defaults: &cwquery.Config{
				Period: longDuration(time.Hour), Lookback: longDuration(6 * time.Hour),
				PublicationDelay: longDuration(30 * time.Minute),
			},
			profile: cwquery.Config{Period: longDuration(5 * time.Minute), PublicationDelay: longDuration(time.Hour)},
			want:    queryPolicy{period: time.Hour, lookback: 6 * time.Hour, publicationDelay: 30 * time.Minute},
		},
		"rule overrides defaults field by field": {
			rule: &cwquery.Config{Period: longDuration(2 * time.Hour), PublicationDelay: longDuration(0)},
			defaults: &cwquery.Config{
				Period: longDuration(time.Hour), Lookback: longDuration(6 * time.Hour),
				PublicationDelay: longDuration(30 * time.Minute),
			},
			profile: cwquery.Config{Period: longDuration(5 * time.Minute), PublicationDelay: longDuration(time.Hour)},
			want:    queryPolicy{period: 2 * time.Hour, lookback: 6 * time.Hour, publicationDelay: 0},
		},
		"omitted lookback follows resolved period": {
			rule:    &cwquery.Config{Period: longDuration(2 * time.Hour)},
			profile: profileQuery(5 * time.Minute),
			want:    queryPolicy{period: 2 * time.Hour, lookback: 2 * time.Hour, publicationDelay: defaultPublicationDelay},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveQueryPolicy("rules[0]", tc.rule, tc.defaults, tc.metric, tc.profile)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveQueryPolicy_Validation(t *testing.T) {
	maxWholeSecondDuration := (time.Duration(1<<63-1) / time.Second) * time.Second
	tests := map[string]struct {
		policy  cwquery.Config
		wantErr string
	}{
		"minimum period":                         {policy: cwquery.Config{Period: longDuration(time.Minute)}},
		"maximum period":                         {policy: cwquery.Config{Period: longDuration(24 * time.Hour)}},
		"period below minimum":                   {policy: cwquery.Config{Period: longDuration(30 * time.Second)}, wantErr: "exact multiple"},
		"period not minute multiple":             {policy: cwquery.Config{Period: longDuration(90 * time.Second)}, wantErr: "exact multiple"},
		"period above maximum":                   {policy: cwquery.Config{Period: longDuration(24*time.Hour + time.Minute)}, wantErr: "between"},
		"lookback below period":                  {policy: cwquery.Config{Period: longDuration(5 * time.Minute), Lookback: longDuration(time.Minute)}, wantErr: "at least"},
		"lookback not period multiple":           {policy: cwquery.Config{Period: longDuration(5 * time.Minute), Lookback: longDuration(6 * time.Minute)}, wantErr: "exact multiple"},
		"exact bucket maximum":                   {policy: cwquery.Config{Period: longDuration(time.Minute), Lookback: longDuration(1440 * time.Minute), PublicationDelay: longDuration(0)}},
		"bucket maximum exceeded":                {policy: cwquery.Config{Period: longDuration(time.Minute), Lookback: longDuration(1441 * time.Minute), PublicationDelay: longDuration(0)}, wantErr: "maximum is 1440"},
		"exact reliable horizon":                 {policy: cwquery.Config{Period: longDuration(time.Hour), Lookback: longDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: longDuration(24 * time.Hour)}},
		"reliable horizon exceeded":              {policy: cwquery.Config{Period: longDuration(time.Hour), Lookback: longDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: longDuration(24*time.Hour + time.Second)}, wantErr: "exceeds 336h0m0s"},
		"duration limit cannot overflow horizon": {policy: cwquery.Config{Period: longDuration(5 * time.Minute), PublicationDelay: longDuration(maxWholeSecondDuration)}, wantErr: "query horizon"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveQueryPolicy("rules[0]", nil, nil, &tc.policy, profileQuery(5*time.Minute))
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Positive(t, got.bucketCount())
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestQueryConfig_RawDurations(t *testing.T) {
	tests := map[string]struct {
		policy  *cwquery.Config
		wantErr string
	}{
		"omitted":             {},
		"explicit zero delay": {policy: &cwquery.Config{PublicationDelay: longDuration(0)}},
		"zero lookback":       {policy: &cwquery.Config{Lookback: longDuration(0)}, wantErr: "must be positive"},
		"negative delay":      {policy: &cwquery.Config{PublicationDelay: longDuration(-time.Second)}, wantErr: "must not be negative"},
		"subsecond lookback":  {policy: &cwquery.Config{Lookback: longDuration(time.Second + time.Millisecond)}, wantErr: "whole seconds"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := cwquery.Validate("rules[0].query", tc.policy)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestQueryPolicy_RecentlyActiveUsesFullHorizon(t *testing.T) {
	tests := map[string]struct {
		policy queryPolicy
		want   bool
	}{
		"exact three-hour horizon": {
			policy: queryPolicy{period: time.Hour, lookback: time.Hour, publicationDelay: time.Hour}, want: true,
		},
		"horizon exceeds three hours": {
			policy: queryPolicy{period: time.Hour, lookback: time.Hour, publicationDelay: time.Hour + time.Second},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.policy.useRecentlyActive())
		})
	}
}
