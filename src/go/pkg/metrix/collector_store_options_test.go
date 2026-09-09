// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCollectorStoreOptions pins the constructor option contract: the retention
// knobs are configurable, and descriptorGraceCycles defaults to the (possibly
// overridden) expireAfterSuccessCycles unless set explicitly.
func TestCollectorStoreOptions(t *testing.T) {
	tests := map[string]struct {
		opts       []CollectorStoreOption
		wantExpire uint64
		wantMax    int
		wantGrace  uint64
	}{
		"defaults couple grace to the default expire": {
			opts:       nil,
			wantExpire: defaultCollectorExpireAfterSuccessCycles,
			wantMax:    defaultCollectorMaxSeries,
			wantGrace:  defaultCollectorExpireAfterSuccessCycles,
		},
		"overriding expire carries into the grace default": {
			opts:       []CollectorStoreOption{WithExpireAfterSuccessCycles(5)},
			wantExpire: 5,
			wantMax:    defaultCollectorMaxSeries,
			wantGrace:  5,
		},
		"explicit grace overrides the coupled default": {
			opts:       []CollectorStoreOption{WithExpireAfterSuccessCycles(5), WithDescriptorGraceCycles(3)},
			wantExpire: 5,
			wantMax:    defaultCollectorMaxSeries,
			wantGrace:  3,
		},
		"explicit zero grace is honored, not treated as unset": {
			opts:       []CollectorStoreOption{WithDescriptorGraceCycles(0)},
			wantExpire: defaultCollectorExpireAfterSuccessCycles,
			wantMax:    defaultCollectorMaxSeries,
			wantGrace:  0,
		},
		"max series is configurable": {
			opts:       []CollectorStoreOption{WithMaxSeries(100)},
			wantExpire: defaultCollectorExpireAfterSuccessCycles,
			wantMax:    100,
			wantGrace:  defaultCollectorExpireAfterSuccessCycles,
		},
		"a nil option is ignored": {
			opts:       []CollectorStoreOption{nil, WithMaxSeries(7)},
			wantExpire: defaultCollectorExpireAfterSuccessCycles,
			wantMax:    7,
			wantGrace:  defaultCollectorExpireAfterSuccessCycles,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := NewCollectorStore(tc.opts...)
			sv := collectorStoreViewForTest(t, s)
			require.Equal(t, tc.wantExpire, sv.core.retention.expireAfterSuccessCycles, "expireAfterSuccessCycles")
			require.Equal(t, tc.wantMax, sv.core.retention.maxSeries, "maxSeries")
			require.Equal(t, tc.wantGrace, sv.core.retention.descriptorGraceCycles, "descriptorGraceCycles")
		})
	}
}
