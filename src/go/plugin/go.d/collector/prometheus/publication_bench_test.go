// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// BenchmarkPublication measures framework publication for a scrape with many
// series per metric name (seriesPerType series of each of the four metric
// types); scrape and write (Collect) are untimed.
func BenchmarkPublication(b *testing.B) {
	for _, seriesPerType := range []int{50, 500} {
		b.Run(fmt.Sprintf("series_per_type_%d", seriesPerType), func(b *testing.B) {
			exposition := buildBenchExposition(seriesPerType)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(exposition))
			}))
			defer srv.Close()
			collr := New()
			collr.URL = srv.URL
			collr.Profiles = ProfilesConfig{
				Mode: profilesModeNone,
			}
			collr.MaxTS = 100 * seriesPerType
			collr.MaxTSPerMetric = 10 * seriesPerType
			require.NoError(b, collr.Init(context.Background()))
			require.NoError(b, collr.Check(context.Background()))

			collecttest.BenchmarkPublication(b, collr, collr.Collect)
		})
	}
}
