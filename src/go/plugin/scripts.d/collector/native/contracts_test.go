// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Use a short, nonzero descriptor grace to observe real retention boundaries.
func contractStore(t *testing.T, c *Collector) metrix.DescriptorRetention {
	t.Helper()
	c.store = metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(2), metrix.WithDescriptorGraceCycles(2))
	return c.store.(metrix.DescriptorRetention)
}
func replaceResponse(t *testing.T, path, response string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path+".next", []byte(response), 0644))
	require.NoError(t, os.Rename(path+".next", path))
}
func responseCollector(t *testing.T) (*Collector, string) {
	t.Helper()
	c, dir := fixtureCollector(t, "cat \"$(dirname \"$0\")/response.json\"\n")
	c.Manifest = ""
	c.Command = []string{filepath.Join(dir, "collect.sh")}
	require.NoError(t, c.Init(context.Background()))
	return c, filepath.Join(dir, "response.json")
}
func responseCycle(t *testing.T, c *Collector, path, response string, failCommit ...bool) error {
	t.Helper()
	replaceResponse(t, path, response)
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	if err := c.Collect(context.Background()); err != nil {
		cycle.AbortCycle()
		return err
	}
	if len(failCommit) > 0 && failCommit[0] {
		injectCommitConflict(c.store)
	}
	return cycle.CommitCycleSuccess()
}
func contractFrame(unit, samples string) string {
	return fmt.Sprintf(`{"version":"v1","metrics":[{"name":"signal","unit":%q,"samples":%s}]}`, unit, samples)
}

func TestContractsFailedAttemptsDoNotEstablish(t *testing.T) {
	setupRunner(t)
	for _, failure := range []string{"collect", "commit"} {
		t.Run(failure, func(t *testing.T) {
			c, path := responseCollector(t)
			store := contractStore(t, c)
			failed := contractFrame("failed-unit", `[{"value":1}]`)
			if failure == "collect" {
				failed = failed[:len(failed)-1] + `,"checks":[{"id":"bad","title":"Bad","samples":[{"state":"invalid"}]}]}`
			}
			require.Error(t, responseCycle(t, c, path, failed, failure == "commit"))
			assert.Equal(t, uint64(0), store.SuccessfulCommits())
			assert.Empty(t, rawSeries(c))
			require.NoError(t, responseCycle(t, c, path, contractFrame("jobs", `[{"value":2}]`)))
			assert.Equal(t, map[string]float64{"signal": 2}, rawSeries(c))
			require.ErrorContains(
				t,
				responseCycle(t, c, path, contractFrame("different", `[{"value":3}]`)),
				"metric contract changed",
			)
			assert.Equal(t, uint64(1), store.SuccessfulCommits())
			assert.Equal(t, map[string]float64{"signal": 2}, rawSeries(c))
		})
	}
}

func TestContractsEmptyFamilyDoesNotEstablish(t *testing.T) {
	setupRunner(t)
	c, path := responseCollector(t)
	contractStore(t, c)
	require.NoError(t, responseCycle(t, c, path, contractFrame("unused", `[]`)))
	_, exists := c.store.Read(metrix.ReadRaw()).MetricMeta("signal")
	assert.False(t, exists)
	require.NoError(t, responseCycle(t, c, path, contractFrame("jobs", `[{"value":3}]`)))
	assert.Equal(t, map[string]float64{"signal": 3}, rawSeries(c))
}

// A live name owns one complete semantic contract, including resolved defaults.
func TestContractsRejectSemanticChanges(t *testing.T) {
	setupRunner(t)
	const scalar = `{"name":"signal","unit":"jobs","chart_meta":{"title":"Queue depth","family":"Queues","priority":42},"samples":[{"value":7}]}`
	const states = `{"name":"signal","type":"stateset","states":["up","down"],"samples":[{"active":["up"]}]}`
	for name, tc := range map[string]struct{ initial, changed string }{
		"kind":             {scalar, strings.Replace(scalar, `"name":"signal"`, `"name":"signal","type":"counter"`, 1)},
		"unit default":     {scalar, strings.Replace(scalar, `"unit":"jobs",`, "", 1)},
		"title":            {scalar, strings.Replace(scalar, "Queue depth", "Renamed", 1)},
		"family":           {scalar, strings.Replace(scalar, "Queues", "Other", 1)},
		"priority":         {scalar, strings.Replace(scalar, `"priority":42`, `"priority":43`, 1)},
		"metadata default": {scalar, strings.Replace(scalar, `"chart_meta":{"title":"Queue depth","family":"Queues","priority":42},`, "", 1)},
		"state domain":     {states, strings.Replace(states, `["up","down"]`, `["up","down","unknown"]`, 1)},
		"state mode":       {states, strings.Replace(states, `"type":"stateset"`, `"type":"stateset","mode":"bitset"`, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			c, path := responseCollector(t)
			require.NoError(t, responseCycle(t, c, path, metricFrame(tc.initial)))
			before := rawSeries(c)
			require.ErrorContains(t, responseCycle(t, c, path, metricFrame(tc.changed)), "metric contract changed")
			assert.Equal(t, before, rawSeries(c))
		})
	}
	c, path := responseCollector(t)
	require.NoError(t, responseCycle(t, c, path, metricFrame(states)))
	reordered := strings.Replace(states, `["up","down"]`, `["down","up"]`, 1)
	require.NoError(
		t,
		responseCycle(t, c, path, metricFrame(reordered)),
		"state declaration order is not a semantic change",
	)
}

func TestContractsRetentionFollowsSuccessfulSamples(t *testing.T) {
	setupRunner(t)
	for _, failure := range []string{"collect", "commit", "empty family"} {
		t.Run(failure, func(t *testing.T) {
			c, path := responseCollector(t)
			store := contractStore(t, c)
			observed := contractFrame("jobs", `[{"value":7}]`)
			require.NoError(t, responseCycle(t, c, path, observed))
			// Reconciliation happens at the next real Collect boundary.
			require.NoError(t, responseCycle(t, c, path, `{"version":"v1"}`))
			require.Contains(t, c.contracts, "signal")
			assert.Equal(t, uint64(1), c.contracts["signal"].lastCommit)
			for range 3 {
				switch failure {
				case "collect":
					require.Error(
						t,
						responseCycle(
							t,
							c,
							path,
							`{"version":"v1","metrics":[{"name":"signal","samples":[{"value":null}]}]}`,
						),
					)
				case "commit":
					require.Error(t, responseCycle(t, c, path, observed, true))
				case "empty family":
					require.NoError(t, responseCycle(t, c, path, contractFrame("jobs", `[]`)))
				}
			}
			if failure != "empty family" {
				assert.Equal(t, uint64(2), store.SuccessfulCommits(), "failed attempts do not age descriptors")
				assert.Equal(
					t,
					uint64(1),
					c.contracts["signal"].lastCommit,
					"failed attempts do not refresh the contract",
				)
			}
			for store.SuccessfulCommits() < 1+store.DescriptorRetentionWindow() {
				require.NoError(t, responseCycle(t, c, path, contractFrame("jobs", `[]`)))
			}
			require.NoError(t, responseCycle(t, c, path, `{"version":"v1"}`))
			assert.NotContains(
				t,
				c.contracts,
				"signal",
				"empty families do not keep validation history alive beyond store retention",
			)
			_, exists := c.store.Read(metrix.ReadRaw()).MetricMeta("signal")
			assert.False(t, exists, "no current sample metadata remains after the advertised retention window")
			require.NoError(t, responseCycle(t, c, path, observed))
			assert.Equal(
				t,
				map[string]float64{"signal": 7},
				rawSeries(c),
				"same-contract return is supported after expiry",
			)
		})
	}
}

func TestContractsRegisteredJobsAreIndependent(t *testing.T) {
	setupRunner(t)
	registry, dir := configuredFixture(t, "read -r config\ncat \"$(dirname \"$0\")/response.json\"\n", modeOneshot)
	path := filepath.Join(dir, "response.json")
	jobs := make([]*Collector, 2)
	for i := range jobs {
		jobs[i] = registry["native-fixture"].CreateV2().(*Collector)
		jobs[i].ScriptConfig = Settings{
			"text": "synthetic",
		}
		require.NoError(t, jobs[i].Init(context.Background()))
	}
	require.NoError(t, responseCycle(t, jobs[0], path, contractFrame("jobs", `[{"value":1}]`)))
	require.NoError(t, responseCycle(t, jobs[1], path, contractFrame("bytes", `[{"value":2}]`)))
	require.NoError(t, responseCycle(t, jobs[0], path, contractFrame("jobs", `[{"value":3}]`)))
	assert.Equal(t, map[string]float64{"signal": 3}, rawSeries(jobs[0]))
	assert.Equal(t, map[string]float64{"signal": 2}, rawSeries(jobs[1]))
	require.NoError(t, responseCycle(t, jobs[0], path, checksFrame(alphaCheck)))
	first := jobs[0].ChartTemplateSet()
	firstEntries := first.Entries()
	require.NoError(t, responseCycle(t, jobs[1], path, checksFrame(betaCheck)))
	assert.Same(t, first, jobs[0].ChartTemplateSet(), "another registered job cannot replace this job's templates")
	assert.Equal(t, firstEntries, jobs[0].ChartTemplateSet().Entries())
	assert.NotEqual(t, jobs[0].ChartTemplateSet().Entries(), jobs[1].ChartTemplateSet().Entries())
	assert.Same(t, jobs[0].definition.templates, jobs[1].definition.templates, "static package definition stays shared")

}

// Deliberately inconsistent host metadata marks the real cycle as failed. It
// never changes the native snapshot or its staged contract data.
func injectCommitConflict(store metrix.CollectorStore) {
	meter := store.Write().SnapshotMeter("test_fault")
	meter.WithHostScope(metrix.HostScope{
		ScopeKey: "fault",
		GUID:     "fault",
		Hostname: "first",
	}).Gauge("value").Observe(1)
	meter.WithHostScope(metrix.HostScope{
		ScopeKey: "fault",
		GUID:     "fault",
		Hostname: "second",
	}).
		Gauge("value").
		Observe(2)
}
