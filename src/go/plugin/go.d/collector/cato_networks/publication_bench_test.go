// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// BenchmarkPublication measures framework publication for the fixture account
// with per-site host scopes; Collect is untimed.
func BenchmarkPublication(b *testing.B) {
	collr, _ := newTestCollector()
	require.NoError(b, collr.Init(context.Background()))

	collecttest.BenchmarkPublication(b, collr, collr.Collect)
}
