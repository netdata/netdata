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
			err := cfg.Deliver(ctx, test.targets, event.Event{Status: "WARNING"}, func(result Result) {
				got = append(got, result)
				cancel()
			})
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, test.want, got)
		})
	}
}
