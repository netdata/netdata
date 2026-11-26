// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// The Agent places a sample by the interval its BEGIN carries and trusts it up
// to five update intervals, so each collector chart's interval must span every
// run since that chart's previous delivered sample. Otherwise its samples, its
// counter rates and the health variable $last_collected_t go wrong after a
// failed cycle, a cycle that did not observe the chart, or a rejected host scope.
func TestJobV2ChartSampleIntervals(t *testing.T) {
	type run struct {
		failed     bool // the collector returns an error
		skipIdle   bool // the idle chart's series are not written
		rejectNode bool // the node scope's output is rejected
	}
	// Runs are ten seconds apart. Keys are "<host>/<chart>"; the default host is "".
	runs := []run{{}, {}, {failed: true}, {skipIdle: true}, {}, {rejectNode: true}, {}}
	want := []map[string]string{
		{"/workers_busy": "", "/workers_idle": "", "node/workers_busy": ""},
		{"/workers_busy": "10000000", "/workers_idle": "10000000", "node/workers_busy": "10000000"},
		{},
		{"/workers_busy": "20000000", "node/workers_busy": "20000000"},
		{"/workers_busy": "10000000", "/workers_idle": "30000000", "node/workers_busy": "10000000"},
		{"/workers_busy": "10000000", "/workers_idle": "10000000"},
		{"/workers_busy": "10000000", "/workers_idle": "10000000", "node/workers_busy": "20000000"},
	}
	node := metrix.HostScope{
		ScopeKey: "node",
		GUID:     sharedGUID,
		Hostname: "node",
	}

	synctest.Test(t, func(t *testing.T) {
		var out bytes.Buffer
		var current run
		output := writeFunc(func(payload []byte) (int, error) {
			if current.rejectNode && bytes.Contains(payload, []byte("HOST '"+sharedGUID+"'")) {
				return 0, errors.New("node output rejected")
			}
			return out.Write(payload)
		})
		store := metrix.NewCollectorStore()
		job := NewJobV2(JobV2Config{
			PluginName: "go.d",
			ModuleName: "test",
			Name:       "job",
			FullName:   "test_job",
			Module: &mockModuleV2{
				store:    store,
				template: chartTemplateV2BusyIdle(),
				collectFunc: func(context.Context) error {
					if current.failed {
						return errors.New("scrape failed")
					}
					meter := store.Write().SnapshotMeter("apache")
					meter.Gauge("workers_busy").Observe(7)
					if !current.skipIdle {
						meter.Gauge("workers_idle").Observe(3)
					}
					meter.WithHostScope(node).Gauge("workers_busy").Observe(5)
					return nil
				},
			},
			Out:         output,
			UpdateEvery: 10,
		})
		require.NoError(t, job.AutoDetectionManaged(context.Background()))

		var got []map[string]string
		for i, r := range runs {
			if i > 0 {
				time.Sleep(10 * time.Second)
			}
			current = r
			out.Reset()
			job.runOnce()
			got = append(got, collectorChartIntervals(out.String(), sharedGUID, "node"))
		}
		assert.Equal(t, want, got)
	})
}

func chartTemplateV2BusyIdle() string {
	return `
version: v1
groups:
  - family: Workers
    metrics:
      - apache.workers_busy
      - apache.workers_idle
    charts:
      - id: workers_busy
        title: Workers Busy
        context: workers_busy
        units: workers
        dimensions:
          - selector: apache.workers_busy
            name: busy
      - id: workers_idle
        title: Workers Idle
        context: workers_idle
        units: workers
        dimensions:
          - selector: apache.workers_idle
            name: idle
`
}

// collectorChartIntervals maps each collector chart BEGIN in the output to its
// interval in microseconds, "" when the BEGIN carries none. Keys are
// "<host>/<chart id>", with the host GUID renamed by alias; job self charts
// (type "netdata") are skipped.
func collectorChartIntervals(wire, guid, alias string) map[string]string {
	intervals := make(map[string]string)
	host := ""
	for line := range strings.SplitSeq(wire, "\n") {
		if rest, ok := strings.CutPrefix(line, "HOST '"); ok {
			host = strings.TrimSuffix(rest, "'")
			if host == guid {
				host = alias
			}
			continue
		}
		rest, ok := strings.CutPrefix(line, "BEGIN '")
		if !ok || strings.HasPrefix(rest, "netdata.") {
			continue
		}
		typeID, interval, _ := strings.Cut(rest, "' ")
		typeID = strings.TrimSuffix(typeID, "'")
		_, chartID, _ := strings.Cut(typeID, ".")
		intervals[host+"/"+chartID] = interval
	}
	return intervals
}
