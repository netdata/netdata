// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const allKindsSnapshot = `{"version":"v1","metrics":[
 {"name":"depth","unit":"jobs","chart_meta":{"title":"Custom queue depth","family":"Queues","priority":1500},"samples":[{"value":1.5,"labels":{"queue":"mail"}}]},
 {"name":"processed_total","type":"counter","unit":"jobs","samples":[{"value":120.25,"labels":{"queue":"mail"}}]},
 {"name":"state","type":"stateset","states":["in progress","done","unknown"],"samples":[{"active":["in progress"]}]},
 {"name":"flags","type":"stateset","mode":"bitset","states":["read","write"],"samples":[{"active":[],"labels":{"queue":"empty"}},{"active":["read","write"],"labels":{"queue":"full"}}]}
]}`

func TestCollector_AllMetricKindsAndAutogen(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "cat \"$(dirname \"$0\")/response.json\"\n")
	c.Manifest = ""
	c.Command = []string{filepath.Join(dir, "collect.sh")}
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "response.json"), []byte(allKindsSnapshot), 0644))
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{
		`depth{queue="mail"}`:                1.5,
		`processed_total{queue="mail"}`:      120.25,
		`state{state="in progress"}`:         1,
		`state{state="done"}`:                0,
		`state{state="unknown"}`:             0,
		`flags{flags="read",queue="empty"}`:  0,
		`flags{flags="write",queue="empty"}`: 0,
		`flags{flags="read",queue="full"}`:   1,
		`flags{flags="write",queue="full"}`:  1,
	}, values)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"native_script.depth":           nil,
			"native_script.processed_total": nil,
			"native_script.state":           nil,
			"native_script.flags":           nil,
		},
	})
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'native_script.flags'") })
	wire := out.String()
	assert.Contains(t, wire, "'Custom queue depth' 'jobs' 'Queues' 'native_script.depth' 'line' '1500'")
	assert.Contains(t, wire, "'incremental'")
	assert.Contains(t, wire, "'absolute'")
	assert.Contains(t, wire, "float")
	assert.Contains(t, wire, "DIMENSION 'in progress' 'in progress' 'absolute'")
	assert.Contains(t, wire, "SET 'in progress' = 1")
	assert.NotContains(t, wire, "'native_script.check_state'", "ordinary StateSets have no built-in alerts")
}

func TestCollector_RejectsUnsafeStateDimensions(t *testing.T) {
	setupRunner(t)
	for name, family := range map[string]string{
		"collision":       `{"name":"state","type":"stateset","states":["up"," up"],"samples":[{"active":[" up"]}]}`,
		"empty dimension": `{"name":"state","type":"stateset","states":["'"],"samples":[{"active":["'"]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, path := responseCollector(t)
			replaceResponse(t, path, metricFrame(`{"name":"depth","samples":[{"value":7}]},`+family))
			out := &wireOutput{}
			job, _ := startTestJob(t, c, out)
			tickUntil(t, job, func() bool {
				wire := out.String()
				return strings.Contains(wire, "SET 'failed' = 1") || strings.Contains(wire, "'native_script.state'")
			})
			job.Stop()
			wire := out.String()
			assert.Contains(t, wire, "SET 'failed' = 1")
			assert.NotContains(t, wire, "'native_script.state'")
			assert.NotContains(t, wire, "'native_script.depth'", "the entire snapshot must fail atomically")
			assert.Empty(t, rawSeries(c))
		})
	}
}

func TestCollector_StateDimensionsPreserveText(t *testing.T) {
	setupRunner(t)
	c, path := responseCollector(t)
	replaceResponse(t, path, metricFrame(
		`{"name":"state","type":"stateset","states":["λ \"in progress\""],"samples":[{"active":["λ \"in progress\""]}]}`,
	))
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'native_script.state'") })
	assert.Contains(t, out.String(), `DIMENSION 'λ "in progress"' 'λ "in progress"' 'absolute'`)
	assert.Contains(t, out.String(), `SET 'λ "in progress"' = 1`)
}

func TestCollector_AuthoredChartsOwnPresentation(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "cat \"$(dirname \"$0\")/response.json\"\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "response.json"), []byte(allKindsSnapshot), 0644))
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'fixture.depth'") })
	assert.Contains(t, out.String(), "'Queue Depth'")
	assert.NotContains(t, out.String(), "'Custom queue depth'")
	assert.NotContains(t, out.String(), "'native_script.depth'")
}
