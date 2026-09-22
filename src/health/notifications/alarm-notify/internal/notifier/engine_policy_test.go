// SPDX-License-Identifier: GPL-3.0-or-later
package notifier

import (
	"context"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanPolicyCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		before  bool
		targets []string
		want    []Result
	}{
		"before filtering":           {before: true, targets: []string{"dev"}},
		"last skipped result":        {targets: []string{"dev"}, want: []Result{{Destination: "dev", SkipReason: "nowarn"}}},
		"remaining skips unreported": {targets: []string{"dev", "after"}, want: []Result{{Destination: "dev", SkipReason: "nowarn"}}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.before {
				cancel()
			}
			cfg := Plan{Routing: Routing{Policies: map[string]*DestinationPolicy{
				"dev": {NoWarn: true}, "after": {NoWarn: true},
			}}}
			var got []Result
			err := cfg.Deliver(ctx, test.targets, Notification{Event: event.Event{Status: "WARNING"}}, func(result Result) {
				got = append(got, result)
				cancel()
			})
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCriticalPolicyMatrix(t *testing.T) {
	const missing = "critical_seen_since_clear is required"
	// Outcomes are ordered as unknown history, never critical, and previously critical.
	for name, test := range map[string]struct {
		policy *DestinationPolicy
		want   map[string][3]string
	}{
		"omitted": {},
		"false":   {policy: &DestinationPolicy{}},
		"nowarn": {policy: &DestinationPolicy{NoWarn: true}, want: map[string][3]string{
			"WARNING": {"nowarn", "nowarn", "nowarn"},
		}},
		"noclear": {policy: &DestinationPolicy{NoClear: true}, want: map[string][3]string{
			"CLEAR": {"noclear", "noclear", "noclear"},
		}},
		"nowarn noclear": {policy: &DestinationPolicy{NoWarn: true, NoClear: true}, want: map[string][3]string{
			"WARNING": {"nowarn", "nowarn", "nowarn"}, "CLEAR": {"noclear", "noclear", "noclear"},
		}},
		"critical": {policy: &DestinationPolicy{Critical: true}, want: map[string][3]string{
			"WARNING": {missing, "critical", ""}, "CLEAR": {missing, "critical", ""},
		}},
		"critical nowarn": {policy: &DestinationPolicy{Critical: true, NoWarn: true}, want: map[string][3]string{
			"WARNING": {"nowarn", "nowarn", "nowarn"}, "CLEAR": {missing, "critical", ""},
		}},
		"critical noclear": {policy: &DestinationPolicy{Critical: true, NoClear: true}, want: map[string][3]string{
			"WARNING": {missing, "critical", ""}, "CLEAR": {"noclear", "noclear", "noclear"},
		}},
		"all": {policy: &DestinationPolicy{Critical: true, NoWarn: true, NoClear: true}, want: map[string][3]string{
			"WARNING": {"nowarn", "nowarn", "nowarn"}, "CLEAR": {"noclear", "noclear", "noclear"},
		}},
	} {
		for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
			for history, fact := range map[string]struct {
				value *bool
				index int
			}{"unknown": {}, "never": {new(false), 1}, "seen": {new(true), 2}} {
				t.Run(name+"/"+status+"/"+history, func(t *testing.T) {
					var calls []string
					var results []Result
					plan := Plan{
						Destinations: map[string]Sender{"target": recordingSender{calls: &calls, name: "target"}},
						Routing:      Routing{Policies: map[string]*DestinationPolicy{"target": test.policy}},
					}
					notification := Notification{Event: event.Event{Status: status}, CriticalSeenSinceClear: fact.value}
					err := plan.Deliver(context.Background(), []string{"target"}, notification, func(result Result) {
						results = append(results, result)
					})
					outcome := test.want[status][fact.index]
					var wantCalls []string
					var wantResults []Result
					if outcome == missing {
						require.ErrorContains(t, err, missing)
					} else {
						require.NoError(t, err)
						wantResults = []Result{{Destination: "target", SkipReason: outcome}}
						if outcome == "" {
							wantCalls = []string{"target:" + status}
						}
					}
					assert.Equal(t, wantCalls, calls)
					assert.Equal(t, wantResults, results)
				})
			}
		}
	}
}

func TestCriticalPolicyLifecycle(t *testing.T) {
	for name, initialFailure := range map[string]bool{"accepted critical": false, "failed critical": true} {
		t.Run(name, func(t *testing.T) {
			var calls []string
			plan := Plan{
				Destinations: map[string]Sender{
					"original": recordingSender{calls: &calls, name: "original"},
					"new":      recordingSender{calls: &calls, name: "new"},
				},
				Routing: Routing{Policies: map[string]*DestinationPolicy{
					"original": {Critical: true}, "new": {Critical: true},
				}},
			}
			for _, step := range []struct {
				status, previous string
				history          bool
				targets          []string
				want             []string
			}{
				{"WARNING", "CLEAR", false, []string{"original"}, nil},
				{"CRITICAL", "WARNING", true, []string{"original"}, []string{"original:CRITICAL"}},
				{"WARNING", "CRITICAL", true, []string{"original", "new"}, []string{"original:WARNING", "new:WARNING"}},
				{"CLEAR", "WARNING", true, []string{"original", "new"}, []string{"original:CLEAR", "new:CLEAR"}},
				{"WARNING", "CLEAR", false, []string{"original", "new"}, nil},
			} {
				calls = nil
				fail := step.status == "CRITICAL" && initialFailure
				plan.Destinations["original"] = recordingSender{calls: &calls, name: "original", fail: fail}
				err := plan.Deliver(context.Background(), step.targets, Notification{
					Event:                  event.Event{Status: step.status, PreviousStatus: step.previous},
					CriticalSeenSinceClear: new(step.history),
				}, func(Result) {})
				if fail {
					require.ErrorContains(t, err, "all attempted destinations failed")
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, step.want, calls, "status %s", step.status)
			}
		})
	}
}
