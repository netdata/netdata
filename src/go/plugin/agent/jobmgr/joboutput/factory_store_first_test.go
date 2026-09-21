// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFactoryStoreFirstRegistrationToWire(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			state := &factoryTestState{}
			store := metrix.NewCollectorStore()
			factory, output := newFactoryTestHarness(t, collectorapi.Creator{
				StoreFirst: enabled,
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state:    state,
						store:    store,
						template: factoryTestChartTemplate,
						collect: func(context.Context) error {
							store.Write().SnapshotMeter("factory").Gauge("value").Observe(1)
							return nil
						},
					}
				},
			}, nil)
			permit, tasks := issueTestJobPermit(t, "module_job", 1)
			prepared, failure, err := prepareFactoryTestCandidate(
				context.Background(), factory, factoryTestConfig(false),
				lifecycle.ResourceIdentity{
					ID:         "module_job",
					Generation: 1,
				}, permit,
			)
			require.NoError(t, err)
			require.Nil(t, failure)
			resource, err := prepared.AcceptStart(context.Background(), 1)
			require.NoError(t, err)
			generation := resource.(*JobGeneration)
			require.NoError(t, generation.Publish())
			require.NoError(t, generation.reserveInstallation())
			require.NoError(t, generation.acknowledgeInstallation())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				assert.NoError(t, generation.Stop(ctx))
				assert.NoError(t, generation.Finalize())
			})

			clock := 0
			require.Eventually(t, func() bool {
				clock++
				generation.resources.candidateJob.Tick(clock)
				return strings.Contains(output.String(), "CHART 'module_job.")
			}, 2*time.Second, 10*time.Millisecond)
			require.NoError(t, generation.Stop(context.Background()))
			require.NoError(t, generation.Finalize())
			requireFactoryAttemptsIdle(t, factory)
			assert.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

			wire := output.String()
			assert.Contains(t, wire, "DIMENSION 'value'")
			assert.Contains(t, wire, "SET 'value' = 1")
			charts := 0
			obsoleteCharts := 0
			for _, line := range strings.Split(wire, "\n") {
				if !strings.HasPrefix(line, "CHART ") {
					continue
				}
				if !strings.HasPrefix(line, "CHART 'module_job.") {
					assert.NotContains(t, line, "store_first", "framework self-metrics retain their defaults")
					continue
				}
				charts++
				options := ""
				if strings.Contains(line, "'obsolete") {
					obsoleteCharts++
					options = "obsolete"
				}
				if enabled {
					options = strings.TrimSpace(options + " store_first")
				}
				assert.Contains(t, line, " '1' '"+options+"' 'test' 'module'")
			}
			assert.Equal(t, 2, charts, "initial and cleanup chart definitions")
			assert.Equal(t, 1, obsoleteCharts)
		})
	}
}
