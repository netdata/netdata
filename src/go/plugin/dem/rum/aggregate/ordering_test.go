// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutOfOrderReceiptsDoNotHideExpiredState(t *testing.T) {
	a, now := newAgg(time.Minute)
	a.Ingest(mk(now.Add(-10*time.Second), "new", "/new", v(beacon.LCP, 10)))
	a.Ingest(mk(now.Add(-2*time.Minute), "old", "/old", v(beacon.LCP, 1000)))
	snap := a.Snapshot()
	assert.Equal(t, 1, snap.Vitals[beacon.LCP].N)
	assert.EqualValues(t, 1, snap.PageviewsWindow)
	assert.Equal(t, 1, snap.ObservedSessions)
	rows, _ := a.Live(0, 100)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		assert.Equal(t, "/new", row.Page)
	}
}
func TestOlderRevisionCannotRefreshWindow(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.CLS, .5))
	b.Vitals[0].Revision = 2
	a.Ingest(b)
	*now = now.Add(50 * time.Second)
	b.Received = *now
	b.Vitals[0].Revision = 1
	b.Vitals[0].Value = .1
	a.Ingest(b)
	*now = now.Add(11 * time.Second)
	assert.Zero(t, a.Snapshot().Vitals[beacon.CLS].N)
}
