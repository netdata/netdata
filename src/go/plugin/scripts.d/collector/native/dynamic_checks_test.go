// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const alphaCheck = `{"id":"alpha","title":"Check Alpha","by_labels":["queue"],"samples":[{"state":"critical","labels":{"queue":"mail"}}]}`
const betaCheck = `{"id":"beta","title":"Check Beta","samples":[{"state":"ok"}]}`

func checksFrame(families ...string) string {
	return `{"version":"v1","checks":[` + strings.Join(families, ",") + `]}`
}
func chartWithTitle(wire, title string) string {
	for line := range strings.SplitSeq(wire, "\n") {
		if strings.HasPrefix(line, "CHART '") && strings.Contains(line, "'"+title+"'") {
			return strings.SplitN(line, "'", 3)[1]
		}
	}
	return ""
}
func obsoleteChart(wire, id string) bool {
	for line := range strings.SplitSeq(wire, "\n") {
		if strings.HasPrefix(line, "CHART '"+id+"'") && strings.Contains(line, "obsolete") {
			return true
		}
	}
	return false
}

func TestDynamicChecksReplaceOnlyChangedFamily(t *testing.T) {
	setupRunner(t)
	c, path := responseCollector(t)
	replaceResponse(t, path, checksFrame(alphaCheck, betaCheck))
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'Check Beta'") })
	first := out.String()
	alphaID, betaID := chartWithTitle(first, "Check Alpha"), chartWithTitle(first, "Check Beta")
	require.NotEmpty(t, alphaID)
	require.NotEmpty(t, betaID)
	changed := strings.ReplaceAll(strings.ReplaceAll(alphaCheck, "Check Alpha", "Renamed Alpha"), "queue", "host")
	replaceResponse(t, path, checksFrame(changed, betaCheck))
	out.Reset()
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'Renamed Alpha'") })
	wire := out.String()
	assert.NotContains(t, wire, "CHART '"+betaID+"'", "unchanged sibling keeps its chart lifecycle")
	assert.Contains(t, wire, "BEGIN '"+betaID+"'")
	assert.Contains(t, wire, "CLABEL 'host' 'mail'")
	assert.False(t, obsoleteChart(wire, betaID))
	renamedID := chartWithTitle(wire, "Renamed Alpha")
	require.NotEmpty(t, renamedID)
	replaceResponse(t, path, checksFrame(betaCheck))
	out.Reset()
	tickUntil(t, job, func() bool { return obsoleteChart(out.String(), renamedID) })
	assert.False(t, obsoleteChart(out.String(), betaID))
	assert.Contains(t, out.String(), "BEGIN '"+betaID+"'")
}

func TestDynamicChecksEmptySnapshotRemovesChecks(t *testing.T) {
	setupRunner(t)
	for name, response := range map[string]string{"omitted": `{"version":"v1"}`, "empty checks": checksFrame(), "empty family": `{"version":"v1","checks":[{"id":"alpha","title":"Check Alpha","by_labels":["queue"],"samples":[]}]}`} {
		t.Run(name, func(t *testing.T) {
			c, path := responseCollector(t)
			replaceResponse(t, path, checksFrame(alphaCheck))
			out := &wireOutput{}
			job, _ := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'critical' = 1") })
			id := chartWithTitle(out.String(), "Check Alpha")
			require.NotEmpty(t, id)
			replaceResponse(t, path, response)
			out.Reset()
			tickUntil(t, job, func() bool { return obsoleteChart(out.String(), id) })
			assert.NotContains(t, out.String(), "SET 'ok' = 1", "disappearance is not recovery")
		})
	}
}

func TestDynamicChecksFailedFramesDoNotPublishDefinitions(t *testing.T) {
	setupRunner(t)
	for _, failure := range []string{"collect", "commit"} {
		t.Run(failure, func(t *testing.T) {
			c, path := responseCollector(t)
			fault := &commitFaultCollector{
				Collector: c,
			}
			if failure == "commit" {
				fault.failCheckTitle = "Failed Alpha"
			}
			failed := checksFrame(strings.Replace(alphaCheck, "Check Alpha", "Failed Alpha", 1))
			if failure == "collect" {
				failed = failed[:len(failed)-1] + `,"metrics":[{"name":"bad","samples":[{"value":null}]}]}`

			}
			replaceResponse(t, path, failed)
			out := &wireOutput{}
			job, _ := startTestJob(t, fault, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'failed' = 1") })
			assert.Equal(t, uint64(0), c.store.(metrix.DescriptorRetention).SuccessfulCommits())
			assert.NotContains(t, out.String(), "Failed Alpha")
			replaceResponse(t, path, checksFrame(betaCheck))
			out.Reset()
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'Check Beta'") })
			assert.NotContains(
				t,
				out.String(),
				"Failed Alpha",
				"the failed candidate must not survive into a complete later frame",
			)
			assert.NotContains(t, out.String(), "native_check_alpha")
		})
	}
}

// Output admission occurs after the real metric commit. Rejecting that frame
// must not make its descriptor's contract mutable on the following attempt.
type rejectNativeOutput struct {
	wireOutput
	rejected atomic.Int64
}

func (out *rejectNativeOutput) CommitJobOutput(payload []byte, state jobruntime.OutputStateTransaction) error {
	if strings.Contains(string(payload), "native_script.signal") {
		out.rejected.Add(1)
		return errors.Join(errors.New("injected output rejection"), state.Abort())
	}
	if _, err := out.Write(payload); err != nil {
		return errors.Join(err, state.Abort())
	}
	return state.Commit()
}
func TestContractsOutputRejectionStillCommitsContract(t *testing.T) {
	setupRunner(t)
	c, path := responseCollector(t)
	replaceResponse(t, path, contractFrame("jobs", `[{"value":7}]`))
	out := &rejectNativeOutput{}
	observed := &contractErrorCollector{
		Collector: c,
		failures:  make(chan error, 8),
	}
	job, _ := startTestJob(t, observed, out)
	tickUntil(t, job, func() bool { return out.rejected.Load() > 0 })
	before := c.store.(metrix.DescriptorRetention).SuccessfulCommits()
	require.Positive(t, before)
	replaceResponse(t, path, contractFrame("changed", `[{"value":9}]`))
	out.Reset()
	tickUntil(t, job, func() bool {
		select {
		case err := <-observed.failures:
			return strings.Contains(err.Error(), "metric contract changed")
		default:
			return false
		}
	})
	job.Stop()
	assert.GreaterOrEqual(t, c.store.(metrix.DescriptorRetention).SuccessfulCommits(), before)
	assert.Equal(t, "jobs", c.contracts["signal"].definition.Unit)
	assert.Equal(t, map[string]float64{"signal": 7}, rawSeries(c))
}

type commitFaultCollector struct {
	*Collector
	failCheckTitle string
}

func (c *commitFaultCollector) Collect(ctx context.Context) error {
	if err := c.Collector.Collect(ctx); err != nil {
		return err
	}
	// Bind the fault to this response. A separately toggled flag can be read
	// before a test replaces the response file, letting the new frame commit.
	for _, check := range c.checks {
		if check.Title == c.failCheckTitle {
			injectCommitConflict(c.store)
			break
		}
	}
	return nil
}

func TestDynamicChecksFailedReplacementPreservesCharts(t *testing.T) {
	setupRunner(t)
	c, path := responseCollector(t)
	fault := &commitFaultCollector{
		Collector:      c,
		failCheckTitle: "Failed replacement",
	}
	original := checksFrame(alphaCheck, betaCheck)
	replaceResponse(t, path, original)
	out := &wireOutput{}
	job, _ := startTestJob(t, fault, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "'Check Beta'") })
	first := out.String()
	alphaID, betaID := chartWithTitle(first, "Check Alpha"), chartWithTitle(first, "Check Beta")
	require.NotEmpty(t, alphaID)
	require.NotEmpty(t, betaID)
	replaceResponse(t, path, checksFrame(strings.Replace(alphaCheck, "Check Alpha", "Failed replacement", 1)))
	out.Reset()
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'failed' = 1") })
	assert.NotContains(t, out.String(), "Failed replacement")
	assert.False(t, obsoleteChart(out.String(), betaID), "failed omission must not obsolete a sibling")
	replaceResponse(t, path, original)
	out.Reset()
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'critical' = 1") })
	assert.NotContains(
		t,
		out.String(),
		"CHART '"+alphaID+"'",
		"failed candidate must not replace the committed template",
	)
	assert.NotContains(t, out.String(), "CHART '"+betaID+"'")
	assert.Contains(t, out.String(), "BEGIN '"+betaID+"'")
}

// Observe actual Collect failures, not the self-metric failure that also follows
// an output rejection. The caller reads collector fields only after Stop joins.
type contractErrorCollector struct {
	*Collector
	failures chan error
}

func (c *contractErrorCollector) Collect(ctx context.Context) error {
	err := c.Collector.Collect(ctx)
	if err != nil {
		select {
		case c.failures <- err:
		default:
		}
	}
	return err
}
