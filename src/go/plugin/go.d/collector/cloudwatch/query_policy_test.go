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
		policy             cwquery.Policy
		wantStart, wantEnd int64
	}{
		"5m period, 10m delay": {policy: cwquery.Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 10 * time.Minute}, wantStart: 999_999_000, wantEnd: 999_999_300},
		"5m period, 2m delay":  {policy: cwquery.Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: 2 * time.Minute}, wantStart: 999_999_300, wantEnd: 999_999_600},
		"daily settled bucket": {policy: cwquery.Policy{Period: 24 * time.Hour, Lookback: 24 * time.Hour, PublicationDelay: 24 * time.Hour}, wantStart: 999_820_800, wantEnd: 999_907_200},
		"15m rolling lookback": {policy: cwquery.Policy{Period: 5 * time.Minute, Lookback: 15 * time.Minute, PublicationDelay: 10 * time.Minute}, wantStart: 999_998_400, wantEnd: 999_999_300},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			start, end := queryWindow(now, tc.policy)
			assert.Equal(t, tc.wantStart, start.Unix(), "start")
			assert.Equal(t, tc.wantEnd, end.Unix(), "end")
			assert.Equal(t, int64(tc.policy.Lookback/time.Second), end.Unix()-start.Unix(), "window length == lookback")
			assert.Zero(t, end.Unix()%int64(tc.policy.Period/time.Second), "end is aligned to a period boundary")
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
	assert.Equal(t, time.Minute, resolved[0].Policy.Period)
	assert.Equal(t, 5*time.Minute, resolved[1].Policy.Period)

	resolved, err = resolveSeriesPolicies("rules[0]", &cwquery.Config{Period: longDuration(2 * time.Minute)}, nil, profile, base)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, resolved[0].Policy.Period)
	assert.Equal(t, 2*time.Minute, resolved[1].Policy.Period)
}

func TestQueryPolicy_RecentlyActiveUsesFullHorizon(t *testing.T) {
	tests := map[string]struct {
		policy cwquery.Policy
		want   bool
	}{
		"exact three-hour horizon": {
			policy: cwquery.Policy{Period: time.Hour, Lookback: time.Hour, PublicationDelay: time.Hour}, want: true,
		},
		"horizon exceeds three hours": {
			policy: cwquery.Policy{Period: time.Hour, Lookback: time.Hour, PublicationDelay: time.Hour + time.Second},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, queryPolicyUsesRecentlyActive(tc.policy))
		})
	}
}
