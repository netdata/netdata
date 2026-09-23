// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskChildContextCarriesApplyDeadlineWithoutCancellation(t *testing.T) {
	plan := time.Unix(2000, 0)
	earlier := time.Unix(1000, 0)
	tests := map[string]struct {
		parent context.Context
		plan   time.Time
		want   time.Time
		wantOK bool
	}{
		"no deadline": {
			parent: context.Background(),
		},
		"plan deadline": {
			parent: context.Background(),
			plan:   plan,
			want:   plan,
			wantOK: true,
		},
		"earlier parent deadline wins": {
			parent: withTestDeadline(t, earlier),
			plan:   plan,
			want:   earlier,
			wantOK: true,
		},
		"parent deadline without a plan deadline": {
			parent: withTestDeadline(t, earlier),
			want:   earlier,
			wantOK: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := newTaskChildContext(test.parent, test.plan)
			defer cancel(nil)
			// Apply runs without cancellation; the deadline must survive that.
			deadline, ok := ApplyDeadline(context.WithoutCancel(ctx))
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.want, deadline)
		})
	}
}

func withTestDeadline(t *testing.T, deadline time.Time) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)
	return ctx
}
