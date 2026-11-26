// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type wireOutput chan string

func (w wireOutput) Write(p []byte) (int, error) {
	select {
	case w <- string(p):
	default:
	}
	return len(p), nil
}

// timedQueryResults are valid results whose processors have spent a quarter
// of the time since the epoch in user mode and the rest idle.
func timedQueryResults() map[string]queryResult {
	results := validQueryResults()
	now := uint64(time.Now().UnixNano() / 100)
	user, idle := now/4, now/4*3
	results[queryCPU] = fixtureRows(
		processorRow("_Total", 0, user, 0, 0, idle),
		processorRow("0,0", 0, user, 0, 0, idle),
		processorRow("0,1", 0, user, 0, 0, idle),
	)
	return results
}

// startJob runs the collector as a V2 job, inside a synctest bubble, and
// returns a function that ticks the job and returns that tick's output.
func startJob(t *testing.T, c *Collector, vnode vnodes.VirtualNode) func(clock int) string {
	t.Helper()
	out := make(wireOutput, 1024)
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "wmi",
		Name:        "fixture",
		ModuleName:  "windows",
		FullName:    "windows_fixture",
		Module:      c,
		Out:         out,
		UpdateEvery: c.UpdateEvery,
		Vnode:       vnode,
		VnodeName:   vnode.Name,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	assert.Equal(t, 0, job.AutoDetectionEvery())

	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		job.StartManaged(run)
	}()
	t.Cleanup(func() {
		run.Stop(nil)
		<-done
		job.Cleanup()
	})
	<-run.StartupDone()
	require.NoError(t, run.StartupErr())

	return func(clock int) string {
		t.Helper()
		synctest.Wait()
		job.Tick(clock)
		synctest.Wait()
		var output strings.Builder
		for len(out) > 0 {
			output.WriteString(<-out)
		}
		return output.String()
	}
}

func TestCollector_JobV2Publication(t *testing.T) {
	const namedGUID = "245dea48-ae2a-4697-a22e-050e7e358afb"
	for name, vnode := range map[string]vnodes.VirtualNode{
		"generated vnode": {},
		"named vnode": {
			Name:     "override",
			Hostname: "configured.example.com",
			GUID:     namedGUID,
		},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c := configuredCollector()
				renameAt := time.Now().Add(inventoryInterval)
				c.Vnode = vnode.Name
				c.newSession = func(context.Context) (querySession, error) {
					results := timedQueryResults()
					hostname := "WIN-LAB"
					if !time.Now().Before(renameAt) {
						hostname = "WIN-RENAMED"
					}
					results[queryOSInfo] = fixtureRows(
						map[string]any{
							"CSName":      hostname,
							"Caption":     "Microsoft Windows Server 2022 Standard",
							"ProductType": uint32(3),
						},
					)
					return &fakeSession{
						results: results,
					}, nil
				}
				collect := startJob(t, c, vnode)

				output := collect(c.UpdateEvery)
				assert.Contains(t, output, "SET 'received' = 190")
				assert.Contains(t, output, "CLABEL 'device' 'Ethernet'")
				assert.NotContains(
					t,
					output,
					"SET 'user'",
					"the first interval starts at the readiness check, at the same instant",
				)
				assert.Contains(t, output, "SET 'used' = 6442450944")
				if vnode.Name != "" {
					assert.Contains(t, output, "HOST_DEFINE '"+namedGUID+"' 'configured.example.com'")
					assert.NotContains(t, output, c.VirtualNode().GUID)
					assert.NotContains(t, output, "HOST_LABEL '_os_edition'")
					return
				}
				guid := c.VirtualNode().GUID
				assert.Contains(t, output, "HOST_DEFINE '"+guid+"' 'WIN-LAB'")
				assert.Contains(t, output, "HOST_LABEL '_vnode_type' 'wmi'")
				assert.Contains(t, output, "HOST_LABEL 'address' 'windows.example.com'")
				assert.Contains(t, output, "HOST_LABEL '_os_edition' 'Standard'")

				time.Sleep(inventoryInterval)
				renamed := collect(2 * c.UpdateEvery)
				assert.Contains(t, renamed, "HOST_DEFINE '"+guid+"' 'WIN-RENAMED'")
				assert.Contains(t, renamed, "SET 'user' = 25")
				assert.Contains(t, renamed, "HOST_LABEL 'address' 'windows.example.com'")
			})
		})
	}
}

type wireDimension struct {
	name       string
	algorithm  string
	multiplier int
	divisor    int
	hidden     bool
}

type wireChart struct {
	context    string
	title      string
	units      string
	chartType  string
	labels     map[string]string
	dimensions []wireDimension
}

var protocolField = regexp.MustCompile(`'([^']*)'|(\S+)`)

// parseCharts reads the chart definitions from plugin protocol output, by
// chart ID. Labels the Agent's protocol reserves (a leading underscore) are
// left out.
func parseCharts(t *testing.T, output string) map[string]*wireChart {
	t.Helper()
	charts := make(map[string]*wireChart)
	var current *wireChart
	for line := range strings.Lines(output) {
		var fields []string
		for _, match := range protocolField.FindAllStringSubmatch(line, -1) {
			fields = append(fields, match[1]+match[2])
		}
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "CHART":
			// CHART type.id name title units family context type priority ...
			require.GreaterOrEqual(t, len(fields), 8, line)
			current = charts[fields[1]]
			if current == nil {
				current = &wireChart{
					labels: make(map[string]string),
				}
				charts[fields[1]] = current
			}
			current.title, current.units, current.context, current.chartType = fields[3], fields[4], fields[6], fields[7]
		case "DIMENSION":
			// DIMENSION id name algorithm multiplier divisor options
			require.NotNil(t, current, line)
			require.Len(t, fields, 7, line)
			multiplier, err := strconv.Atoi(fields[4])
			require.NoError(t, err)
			divisor, err := strconv.Atoi(fields[5])
			require.NoError(t, err)
			dim := wireDimension{
				name:       fields[2],
				algorithm:  fields[3],
				multiplier: multiplier,
				divisor:    divisor,
				hidden:     slices.Contains(strings.Fields(fields[6]), "hidden"),
			}
			if i := slices.IndexFunc(current.dimensions, func(d wireDimension) bool { return d.name == dim.name }); i >= 0 {
				current.dimensions[i] = dim
			} else {
				current.dimensions = append(current.dimensions, dim)
			}
		case "CLABEL":
			require.NotNil(t, current, line)
			require.GreaterOrEqual(t, len(fields), 3, line)
			if !strings.HasPrefix(fields[1], "_") {
				current.labels[fields[1]] = fields[2]
			}
		}
	}
	return charts
}

// publishedCharts runs two collections through the V2 job path, so the CPU
// charts have an interval, and returns every chart the job defined.
func publishedCharts(t *testing.T) []wireChart {
	t.Helper()
	var charts []wireChart
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Vnode = "fixture"
		c.newSession = func(context.Context) (querySession, error) {
			return &fakeSession{
				results: timedQueryResults(),
			}, nil
		}
		collect := startJob(t, c, vnodes.VirtualNode{
			Name:     "fixture",
			Hostname: "fixture",
			GUID:     "245dea48-ae2a-4697-a22e-050e7e358afb",
		})
		output := collect(c.UpdateEvery)
		time.Sleep(time.Duration(c.UpdateEvery) * time.Second)
		output += collect(2 * c.UpdateEvery)
		for _, chart := range parseCharts(t, output) {
			// The job runtime's own collection charts are not the collector's.
			if !strings.HasPrefix(chart.context, "netdata.") {
				charts = append(charts, *chart)
			}
		}
	})
	return charts
}

// TestCollector_PublishesNativeWindowsCharts compares the published charts with
// the native Windows Agent's definitions of the same contexts, the oracle:
// src/collectors/windows.plugin/perflib-processor.c (system.cpu, cpu.cpu),
// src/collectors/common-contexts/system-ram.h (system.ram),
// src/collectors/windows.plugin/perflib-network.c (net.net) and
// src/collectors/windows.plugin/perflib-storage.c (disk.space). Native CPU
// dimensions use the percent-of-total-delta algorithm, which this collector
// computes itself, so they arrive as absolute percentages. Families are
// display metadata and not compared.
func TestCollector_PublishesNativeWindowsCharts(t *testing.T) {
	modes := []wireDimension{
		{name: "irq", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "user", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "system", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "dpc", algorithm: "absolute", multiplier: 1, divisor: 1},
		{name: "idle", algorithm: "absolute", multiplier: 1, divisor: 1, hidden: true},
	}
	native := map[string]wireChart{
		"system.cpu": {
			title:      "Total CPU Utilization",
			units:      "percentage",
			chartType:  "stacked",
			dimensions: modes,
		},
		"cpu.cpu": {
			title:      "Core Utilization",
			units:      "percentage",
			chartType:  "stacked",
			dimensions: modes,
		},
		"system.ram": {
			title:     "System RAM",
			units:     "MiB",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "free", algorithm: "absolute", multiplier: 1, divisor: 1048576},
				{name: "used", algorithm: "absolute", multiplier: 1, divisor: 1048576},
			},
		},
		"net.net": {
			title:     "Bandwidth",
			units:     "kilobits/s",
			chartType: "area",
			dimensions: []wireDimension{
				{name: "received", algorithm: "incremental", multiplier: 8, divisor: 1000},
				{name: "sent", algorithm: "incremental", multiplier: -8, divisor: 1000},
			},
		},
		"disk.space": {
			title:     "Disk Space Usage",
			units:     "GiB",
			chartType: "stacked",
			dimensions: []wireDimension{
				{name: "avail", algorithm: "absolute", multiplier: 1, divisor: 1073741824},
				{name: "used", algorithm: "absolute", multiplier: 1, divisor: 1073741824},
			},
		},
	}
	instance := func(context string, labels map[string]string) wireChart {
		chart := native[context]
		chart.context = context
		chart.labels = labels
		return chart
	}

	// One chart per instance in the fixture; the interface without traffic has none.
	assert.ElementsMatch(t, []wireChart{
		instance("system.cpu", map[string]string{}),
		instance("cpu.cpu", map[string]string{"cpu": "cpu0"}),
		instance("cpu.cpu", map[string]string{"cpu": "cpu1"}),
		instance("system.ram", map[string]string{}),
		instance("net.net", map[string]string{"device": "Ethernet", "interface_type": "real"}),
		instance("net.net", map[string]string{"device": "Ethernet#1", "interface_type": "real"}),
		instance("disk.space", map[string]string{
			"mount_point":   "C:",
			"drive_type":    "fixed",
			"filesystem":    "NTFS",
			"rw_mode":       "rw",
			"serial_number": "0xBEF3E296",
		}),
		instance("disk.space", map[string]string{
			"mount_point":   "D:",
			"drive_type":    "fixed",
			"filesystem":    "unknown",
			"rw_mode":       "rw",
			"serial_number": "0xABCD",
		}),
	}, publishedCharts(t))
}

// TestMetadataDocumentsChartLabels requires every metadata scope to document
// exactly the labels the job publishes on the charts of its metrics.
func TestMetadataDocumentsChartLabels(t *testing.T) {
	published := make(map[string][]string)
	for _, chart := range publishedCharts(t) {
		keys := slices.AppendSeq([]string{}, maps.Keys(chart.labels))
		slices.Sort(keys)
		if previous, ok := published[chart.context]; ok {
			require.Equal(t, previous, keys, "labels of %s instances", chart.context)
		}
		published[chart.context] = keys
	}

	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var doc struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Labels []struct {
						Name string `yaml:"name"`
					} `yaml:"labels"`
					Metrics []struct {
						Name string `yaml:"name"`
					} `yaml:"metrics"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Modules, 1)

	documented := make(map[string][]string)
	for _, scope := range doc.Modules[0].Metrics.Scopes {
		labels := []string{}
		for _, label := range scope.Labels {
			labels = append(labels, label.Name)
		}
		slices.Sort(labels)
		for _, metric := range scope.Metrics {
			documented[metric.Name] = labels
		}
	}
	assert.Equal(t, published, documented)
}
