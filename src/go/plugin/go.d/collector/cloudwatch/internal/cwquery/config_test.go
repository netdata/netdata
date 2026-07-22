// SPDX-License-Identifier: GPL-3.0-or-later

package cwquery

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDuration(value time.Duration) *confopt.LongDuration {
	v := confopt.LongDuration(value)
	return &v
}

func testProfileQuery(period time.Duration) Config {
	return Config{Period: testDuration(period)}
}

func resolveForTest(rule, defaults, metric *Config, profile Config) (Policy, error) {
	return Resolve(Resolution{
		Path:         "rules[0]",
		Profile:      Source{Config: &profile, Path: "profile.query"},
		Metric:       Source{Config: metric, Path: "profile.metrics[0].query"},
		RuleDefaults: Source{Config: defaults, Path: "rule_defaults.query"},
		Rule:         Source{Config: rule, Path: "rules[0].query"},
	})
}

func TestResolve_Precedence(t *testing.T) {
	tests := map[string]struct {
		rule, defaults *Config
		metric         *Config
		profile        Config
		want           Policy
	}{
		"profile and built-in fallbacks": {
			profile: testProfileQuery(5 * time.Minute),
			want:    Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: DefaultPublicationDelay},
		},
		"profile publication delay": {
			profile: Config{Period: testDuration(5 * time.Minute), PublicationDelay: testDuration(time.Hour)},
			want:    Policy{Period: 5 * time.Minute, Lookback: 5 * time.Minute, PublicationDelay: time.Hour},
		},
		"metric overrides profile field by field": {
			metric:  &Config{Period: testDuration(time.Hour), Lookback: testDuration(6 * time.Hour), PublicationDelay: testDuration(30 * time.Minute)},
			profile: Config{Period: testDuration(5 * time.Minute), Lookback: testDuration(10 * time.Minute), PublicationDelay: testDuration(time.Hour)},
			want:    Policy{Period: time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 30 * time.Minute},
		},
		"rule defaults override profile field by field": {
			defaults: &Config{
				Period: testDuration(time.Hour), Lookback: testDuration(6 * time.Hour),
				PublicationDelay: testDuration(30 * time.Minute),
			},
			profile: Config{Period: testDuration(5 * time.Minute), PublicationDelay: testDuration(time.Hour)},
			want:    Policy{Period: time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 30 * time.Minute},
		},
		"rule overrides defaults field by field": {
			rule: &Config{Period: testDuration(2 * time.Hour), PublicationDelay: testDuration(0)},
			defaults: &Config{
				Period: testDuration(time.Hour), Lookback: testDuration(6 * time.Hour),
				PublicationDelay: testDuration(30 * time.Minute),
			},
			profile: Config{Period: testDuration(5 * time.Minute), PublicationDelay: testDuration(time.Hour)},
			want:    Policy{Period: 2 * time.Hour, Lookback: 6 * time.Hour, PublicationDelay: 0},
		},
		"omitted lookback follows resolved period": {
			rule:    &Config{Period: testDuration(2 * time.Hour)},
			profile: testProfileQuery(5 * time.Minute),
			want:    Policy{Period: 2 * time.Hour, Lookback: 2 * time.Hour, PublicationDelay: DefaultPublicationDelay},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveForTest(tc.rule, tc.defaults, tc.metric, tc.profile)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolve_Validation(t *testing.T) {
	maxWholeSecondDuration := (time.Duration(1<<63-1) / time.Second) * time.Second
	tests := map[string]struct {
		policy  Config
		wantErr string
	}{
		"minimum period":                         {policy: Config{Period: testDuration(time.Minute)}},
		"maximum period":                         {policy: Config{Period: testDuration(24 * time.Hour)}},
		"period below minimum":                   {policy: Config{Period: testDuration(30 * time.Second)}, wantErr: "exact multiple"},
		"period not minute multiple":             {policy: Config{Period: testDuration(90 * time.Second)}, wantErr: "exact multiple"},
		"period above maximum":                   {policy: Config{Period: testDuration(24*time.Hour + time.Minute)}, wantErr: "between"},
		"lookback below period":                  {policy: Config{Period: testDuration(5 * time.Minute), Lookback: testDuration(time.Minute)}, wantErr: "at least"},
		"lookback not period multiple":           {policy: Config{Period: testDuration(5 * time.Minute), Lookback: testDuration(6 * time.Minute)}, wantErr: "exact multiple"},
		"exact bucket maximum":                   {policy: Config{Period: testDuration(time.Minute), Lookback: testDuration(1440 * time.Minute), PublicationDelay: testDuration(0)}},
		"bucket maximum exceeded":                {policy: Config{Period: testDuration(time.Minute), Lookback: testDuration(1441 * time.Minute), PublicationDelay: testDuration(0)}, wantErr: "maximum is 1440"},
		"exact reliable horizon":                 {policy: Config{Period: testDuration(time.Hour), Lookback: testDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: testDuration(24 * time.Hour)}},
		"reliable horizon exceeded":              {policy: Config{Period: testDuration(time.Hour), Lookback: testDuration(12*24*time.Hour + 23*time.Hour), PublicationDelay: testDuration(24*time.Hour + time.Second)}, wantErr: "exceeds 336h0m0s"},
		"duration limit cannot overflow horizon": {policy: Config{Period: testDuration(5 * time.Minute), PublicationDelay: testDuration(maxWholeSecondDuration)}, wantErr: "query horizon"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := resolveForTest(nil, nil, &tc.policy, testProfileQuery(5*time.Minute))
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Positive(t, got.BucketCount())
				return
			}
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidate_RawDurations(t *testing.T) {
	tests := map[string]struct {
		policy  *Config
		wantErr string
	}{
		"omitted":             {},
		"explicit zero delay": {policy: &Config{PublicationDelay: testDuration(0)}},
		"zero lookback":       {policy: &Config{Lookback: testDuration(0)}, wantErr: "must be positive"},
		"negative delay":      {policy: &Config{PublicationDelay: testDuration(-time.Second)}, wantErr: "must not be negative"},
		"subsecond lookback":  {policy: &Config{Lookback: testDuration(time.Second + time.Millisecond)}, wantErr: "whole seconds"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := Validate("rules[0].query", tc.policy)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestResolve_ReportsSelectedSourcePath(t *testing.T) {
	tests := map[string]struct {
		rule, defaults *Config
		metric         *Config
		profile        Config
		wantPath       string
	}{
		"profile": {
			profile:  Config{Period: testDuration(90 * time.Second)},
			wantPath: "profile.query.period",
		},
		"metric": {
			metric:   &Config{Lookback: testDuration(6 * time.Minute)},
			profile:  testProfileQuery(5 * time.Minute),
			wantPath: "profile.metrics[0].query.lookback",
		},
		"rule defaults": {
			defaults: &Config{Lookback: testDuration(6 * time.Minute)},
			profile:  testProfileQuery(5 * time.Minute),
			wantPath: "rule_defaults.query.lookback",
		},
		"rule": {
			rule:     &Config{Lookback: testDuration(6 * time.Minute)},
			profile:  testProfileQuery(5 * time.Minute),
			wantPath: "rules[0].query.lookback",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := resolveForTest(tc.rule, tc.defaults, tc.metric, tc.profile)

			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantPath)
		})
	}
}
