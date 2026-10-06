// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoryRange(t *testing.T) {
	for _, tc := range []struct {
		name               string
		args               map[string]string
		now, after, before int64
		invalid            bool
	}{
		{name: "omitted", now: 2000, after: 1100, before: 2000},
		{name: "epoch both", args: map[string]string{"after": "0", "before": "0"}, now: 2000},
		{name: "epoch after", args: map[string]string{"after": "0"}, now: 2000, before: 2000},
		{name: "epoch before", args: map[string]string{"before": "0"}, now: 2000},
		{name: "clamp default", args: map[string]string{"before": "100"}, now: 2000, before: 100},
		{name: "relative same now", args: map[string]string{"after": "-120", "before": "-60"}, now: 2000, after: 1880, before: 1940},
		{name: "relative epoch", args: map[string]string{"after": "-2000"}, now: 2000, before: 2000},
		{name: "pre epoch after", args: map[string]string{"after": "-2001"}, now: 2000, invalid: true},
		{name: "pre epoch before", args: map[string]string{"before": "-2001"}, now: 2000, invalid: true},
		{name: "inverted", args: map[string]string{"after": "1", "before": "0"}, now: 2000, invalid: true},
		{name: "invalid", args: map[string]string{"after": "bad"}, now: 2000, invalid: true},
		{name: "overflow", args: map[string]string{"before": "9223372036854775808"}, now: 2000, invalid: true},
		{name: "extreme relative", args: map[string]string{"after": "-9223372036854775808"}, now: 2000, invalid: true},
		{name: "max seconds", args: map[string]string{"after": "0", "before": "9223372036854775807"}, now: 2000, before: math.MaxInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, before, err := historyRange(tc.args, tc.now)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.after, after)
			require.Equal(t, tc.before, before)
		})
	}
}
