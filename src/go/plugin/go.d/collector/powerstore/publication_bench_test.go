// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// BenchmarkPublication measures framework publication for the fixture storage
// array; Collect (HTTP) is untimed.
func BenchmarkPublication(b *testing.B) {
	srv := newMockPowerStoreServer()
	defer srv.Close()
	collr := New()
	collr.Config = Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Username: "admin",
				Password: "password",
			},
		},
	}
	require.NoError(b, collr.Init(context.Background()))
	require.NoError(b, collr.Check(context.Background()))

	collecttest.BenchmarkPublication(b, collr, collr.Collect)
}
