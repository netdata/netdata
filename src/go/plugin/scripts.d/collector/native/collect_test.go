// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_Collect(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "[[ $1 == collect ]]\ncat \"$(dirname \"$0\")/response.json\"\n")
	file := filepath.Join(dir, "response.json")
	for _, state := range []string{"critical", "warning", "unknown", "ok"} {
		t.Run(state, func(t *testing.T) {
			require.NoError(t, os.WriteFile(file, []byte(snapshotJSON(state)), 0644))
			values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			want := map[string]float64{
				`depth{queue="mail",region="east"}`: 17,
				`processed_total{queue="mail"}`:     100,
			}
			for _, candidate := range checkStates {
				value := float64(0)
				if state == candidate {
					value = 1
				}
				want[`native.check.backlog{native.check.backlog="`+candidate+`",queue="mail"}`] = value
			}
			assert.Equal(t, want, values)
			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
				RequiredContexts: map[string][]string{
					"fixture.depth":             {"depth"},
					"fixture.processed":         {"processed"},
					"native_script.check_state": {"ok", "warning", "critical", "unknown"},
				},
			})
		})
	}
	// A valid empty snapshot means disappearance; it must not replay the last OK.
	require.NoError(t, os.WriteFile(file, []byte(`{"version":"v1","metrics":[],"checks":[]}`), 0644))
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, values)
}

func TestCollector_CollectInvalidSnapshotWritesNothing(t *testing.T) {
	setupRunner(t)
	c, _ := fixtureCollector(
		t,
		`printf '%s\n' '{"version":"v1","metrics":[{"name":"depth","value":123},{"name":"missing","value":1}]}'`+"\n",
	)
	// Commit deliberately to prove the parser staged nothing, even before the
	// production runtime's additional AbortCycle protection.
	require.Error(t, collectAndCommit(t, c))
	assert.Empty(t, rawSeries(c))
}

func TestCollector_CollectCheckWithoutIdentityLabels(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, `printf '%s' '{"version":"v1","checks":[{"id":"probe","state":"critical"}]}'`+"\n")
	manifest := "version: v1\ncommand: [./collect.sh]\nchecks:\n  - {id: probe, title: Probe}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644))
	require.NoError(t, c.Init(context.Background()))
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"native_script.check_state": {"critical", "ok", "warning", "unknown"},
		},
	})
}
