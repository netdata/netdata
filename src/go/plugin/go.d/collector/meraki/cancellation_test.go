// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitRejectsCanceledContext(t *testing.T) {
	c := New()
	c.OrganizationID = "org"
	c.APIKey = "test-key"
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Init(ctx), context.Canceled)
}

type cancelingGauge struct {
	metrix.SnapshotGauge
	cancel context.CancelFunc
}

func (g cancelingGauge) Observe(value metrix.SampleValue, labels ...metrix.LabelSet) {
	g.SnapshotGauge.Observe(value, labels...)
	g.cancel()
}

func TestCollectAbortsCancellationDuringMetricWrites(t *testing.T) {
	_, server := newFixtureAPI(t)
	c := newTestCollector(t, server)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Delegate to the real store, then cancel while the cycle is still staging
	// observations. A canceled collection must abort instead of publishing them.
	c.metrics.organizationAssigned = cancelingGauge{SnapshotGauge: c.metrics.organizationAssigned, cancel: cancel}
	require.ErrorIs(t, collectCycle(ctx, c), context.Canceled)
	assert.Empty(t, readOrganization(c))
	assert.Empty(t, readDevices(c))
}
