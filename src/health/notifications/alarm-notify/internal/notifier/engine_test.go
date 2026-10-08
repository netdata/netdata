// SPDX-License-Identifier: GPL-3.0-or-later
package notifier

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingSender struct {
	calls *[]string
	name  string
	fail  bool
}

func (s recordingSender) Send(_ context.Context, e event.Event) error {
	*s.calls = append(*s.calls, s.name+":"+e.Status)
	if s.fail {
		return errors.New("delivery rejected")
	}
	return nil
}

func TestPlanDelivery(t *testing.T) {
	for name, test := range map[string]struct {
		names       []string
		fail        map[string]bool
		policies    map[string]*DestinationPolicy
		wantCalls   []string
		wantSkipped []string
		err         string
	}{
		"empty":               {},
		"ordered":             {names: []string{"first", "second"}, wantCalls: []string{"first:WARNING", "second:WARNING"}},
		"failed then success": {names: []string{"first", "second"}, fail: map[string]bool{"first": true}, wantCalls: []string{"first:WARNING", "second:WARNING"}},
		"all fail":            {names: []string{"first", "second"}, fail: map[string]bool{"first": true, "second": true}, wantCalls: []string{"first:WARNING", "second:WARNING"}, err: "all attempted destinations failed"},
		"skipped":             {names: []string{"first"}, policies: map[string]*DestinationPolicy{"first": {NoWarn: true}}, wantSkipped: []string{"first:nowarn"}},
		"skip and failure":    {names: []string{"first", "second"}, fail: map[string]bool{"second": true}, policies: map[string]*DestinationPolicy{"first": {NoWarn: true}}, wantCalls: []string{"second:WARNING"}, wantSkipped: []string{"first:nowarn"}, err: "all attempted destinations failed"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls, skipped []string
			plan := Plan{Destinations: map[string]Sender{}, Routing: Routing{Policies: test.policies}}
			for _, name := range test.names {
				plan.Destinations[name] = recordingSender{&calls, name, test.fail[name]}
			}
			results := 0
			err := plan.Deliver(context.Background(), test.names, Notification{Event: event.Event{Status: "WARNING"}}, func(r Result) {
				results++
				if r.SkipReason != "" {
					skipped = append(skipped, r.Destination+":"+r.SkipReason)
					require.NoError(t, r.Err)
				} else {
					assert.Equal(t, test.fail[r.Destination], r.Err != nil)
				}
			})
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.wantCalls, calls)
			assert.Equal(t, test.wantSkipped, skipped)
			assert.Equal(t, len(test.names), results)
		})
	}
}
