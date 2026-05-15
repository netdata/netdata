// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	specYAML, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, specYAML.Validate())
	_, err = chartengine.Compile(specYAML, 1)
	require.NoError(t, err)

	tests := map[string]struct {
		context   string
		selector  string
		wantFloat bool
	}{
		"execution duration dimension is float": {
			context:   "execution_duration",
			selector:  "nagios.job.execution_duration",
			wantFloat: true,
		},
		"execution cpu dimension is float": {
			context:   "execution_cpu",
			selector:  "nagios.job.execution_cpu_total",
			wantFloat: true,
		},
		"execution memory dimension is integer": {
			context:   "execution_memory",
			selector:  "nagios.job.execution_max_rss",
			wantFloat: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dim, ok := findChartDimensionByContext(specYAML, tc.context)
			require.True(t, ok, "missing chart context %q", tc.context)
			assert.Equal(t, tc.selector, dim.Selector)
			if tc.wantFloat {
				require.NotNil(t, dim.Options)
				assert.True(t, dim.Options.Float)
				return
			}
			if dim.Options != nil {
				assert.False(t, dim.Options.Float)
			}
		})
	}
}

func TestCollector_ConfigSchema(t *testing.T) {
	tests := map[string]struct {
		assert func(*testing.T, nagiosConfigSchemaDoc)
	}{
		"wrapped schema follows collector conventions": {
			assert: func(t *testing.T, doc nagiosConfigSchemaDoc) {
				t.Helper()
				assert.NotEmpty(t, doc.JSONSchema.Schema)
				_, hasPlugin := doc.JSONSchema.Properties["plugin"]
				_, hasCheckName := doc.JSONSchema.Properties["check_name"]
				_, hasName := doc.JSONSchema.Properties["name"]
				_, hasTimeoutState := doc.JSONSchema.Properties["timeout_state"]
				_, hasUIOptions := doc.UISchema["uiOptions"]
				assert.True(t, hasPlugin)
				assert.True(t, hasCheckName)
				assert.False(t, hasName)
				assert.False(t, hasTimeoutState)
				assert.True(t, hasUIOptions)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var doc nagiosConfigSchemaDoc
			require.NoError(t, json.Unmarshal([]byte(configSchema), &doc))
			tc.assert(t, doc)
		})
	}
}

func TestCollector_New(t *testing.T) {
	tests := map[string]struct {
		assert func(*testing.T, *Collector)
	}{
		"exposes runtime defaults on the live collector config": {
			assert: func(t *testing.T, coll *Collector) {
				t.Helper()
				assert.Equal(t, defaultCollectorUpdateEvery, coll.Config.UpdateEvery)
				assert.Equal(t, confDuration(5*time.Second), coll.Config.JobConfig.Timeout)
				assert.Equal(t, confDuration(5*time.Minute), coll.Config.JobConfig.CheckInterval)
				assert.Equal(t, confDuration(1*time.Minute), coll.Config.JobConfig.RetryInterval)
				assert.Equal(t, 3, coll.Config.JobConfig.MaxCheckAttempts)
				assert.Equal(t, "24x7", coll.Config.JobConfig.CheckPeriod)
				require.NotNil(t, coll.Config.JobConfig.Environment)
				require.NotNil(t, coll.Config.JobConfig.CustomVars)
				assert.Empty(t, coll.Config.JobConfig.Environment)
				assert.Empty(t, coll.Config.JobConfig.CustomVars)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			coll := New()
			tc.assert(t, coll)
		})
	}
}

type nagiosConfigSchemaDoc struct {
	JSONSchema struct {
		Schema     string                     `json:"$schema"`
		Properties map[string]json.RawMessage `json:"properties"`
	} `json:"jsonSchema"`
	UISchema map[string]json.RawMessage `json:"uiSchema"`
}

func TestCollector_Check(t *testing.T) {
	truePluginPath := writeTestPluginFile(t, "true")

	tests := map[string]struct {
		config   Config
		wantErr  bool
		errMatch string
	}{
		"missing plugin": {
			config:   Config{JobConfig: JobConfig{Name: "invalid-without-plugin"}},
			wantErr:  true,
			errMatch: "plugin path is required",
		},
		"update_every exceeds cadence": {
			config: Config{
				UpdateEvery: 10,
				JobConfig: JobConfig{
					Name:          "cadence",
					Plugin:        truePluginPath,
					CheckInterval: confDuration(5 * time.Second),
					RetryInterval: confDuration(5 * time.Second),
				},
			},
			wantErr: false,
		},
		"valid config": {
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:          "valid",
					Plugin:        truePluginPath,
					CheckInterval: confDuration(5 * time.Second),
					RetryInterval: confDuration(5 * time.Second),
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			coll := newTestCollector()
			coll.runner = &fakeRunner{}
			coll.Config = tc.config

			err := coll.Check(context.Background())
			if tc.wantErr {
				require.Error(t, err)
				if tc.errMatch != "" {
					assert.Contains(t, err.Error(), tc.errMatch)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsKnownInterpreter(t *testing.T) {
	tests := map[string]struct {
		path string
		want bool
	}{
		"bash":              {path: "/bin/bash", want: true},
		"sh":                {path: "/bin/sh", want: true},
		"python3":           {path: "/usr/bin/python3", want: true},
		"python3 versioned": {path: "/usr/bin/python3.11", want: true},
		"powershell":        {path: "/usr/bin/powershell", want: true},
		"powershell.exe":    {path: "/powershell.exe", want: true},
		"pwsh":              {path: "/usr/bin/pwsh", want: true},
		"env":               {path: "/usr/bin/env", want: true},
		"node":              {path: "/usr/bin/node", want: true},
		"cmd.exe":           {path: "/cmd.exe", want: true},
		"php":               {path: "/usr/bin/php", want: true},
		"check_ping":        {path: "/usr/lib/nagios/plugins/check_ping", want: false},
		"check_http":        {path: "/usr/lib/nagios/plugins/check_http", want: false},
		"custom script":     {path: "/opt/netdata/checks/check_api.sh", want: false},
		"custom exe":        {path: "/opt/checks/check_service.exe", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isKnownInterpreter(tc.path))
		})
	}
}

func TestCompileCollectorConfig_CadenceWarning(t *testing.T) {
	tests := map[string]struct {
		config      Config
		wantErr     bool
		wantWarning bool
	}{
		"warning when update_every exceeds retry interval": {
			config: Config{
				UpdateEvery: 10,
				JobConfig: JobConfig{
					Name:          "cadence-warning",
					Plugin:        "/bin/true",
					CheckInterval: confDuration(10 * time.Second),
					RetryInterval: confDuration(2 * time.Second),
				},
			},
			wantWarning: true,
		},
		"no warning when cadence fits update_every": {
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:          "cadence-ok",
					Plugin:        "/bin/true",
					CheckInterval: confDuration(5 * time.Second),
					RetryInterval: confDuration(5 * time.Second),
				},
			},
		},
		"invalid config still fails": {
			config: Config{
				JobConfig: JobConfig{
					Name: "invalid",
				},
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			job, err := compileCollectorConfig(tc.config)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantWarning, job.cadenceWarning != "", "warning: %q", job.cadenceWarning)
		})
	}
}

func TestCollector_Init(t *testing.T) {
	truePluginPath := writeTestPluginFile(t, "true")

	tests := map[string]struct {
		config Config
		assert func(*testing.T, *Collector)
	}{
		"default timing comes from spec": {
			config: Config{
				JobConfig: JobConfig{
					Name:   "defaults",
					Plugin: truePluginPath,
				},
			},
			assert: func(t *testing.T, coll *Collector) {
				t.Helper()
				assert.Equal(t, confDuration(5*time.Minute), coll.job.config.CheckInterval)
				assert.Equal(t, confDuration(1*time.Minute), coll.job.config.RetryInterval)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			coll := newTestCollector()
			coll.runner = &fakeRunner{}
			coll.Config = tc.config
			require.NoError(t, coll.Init(context.Background()))
			tc.assert(t, coll)
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	truePluginPath := writeTestPluginFile(t, "true")
	pwshPluginPath := writeTestPluginFile(t, "pwsh")

	tests := map[string]struct {
		results []fakeRun
		config  Config
		setup   func(*Collector, *fakeRunner, *time.Time)
		run     func(*testing.T, *Collector, *fakeRunner, *time.Time)
	}{
		"replays cached metrics when not due": {
			results: []fakeRun{
				{
					result: checkRunResult{
						ServiceState: "OK",
						JobState:     "OK",
						Duration:     2500 * time.Millisecond,
						Usage: ndexec.ResourceUsage{
							User:        300 * time.Millisecond,
							System:      200 * time.Millisecond,
							MaxRSSBytes: 12345,
						},
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								{Label: "used", Unit: "KB", Value: 30},
							},
						},
					},
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:          "check_disk",
					Plugin:        truePluginPath,
					CheckInterval: confDuration(5 * time.Minute),
					RetryInterval: confDuration(1 * time.Minute),
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				read := coll.MetricStore().Read(metrix.ReadRaw())
				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "check_disk", "nagios.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "check_disk", "nagios.perfdata.true.job.execution_state": "ok"}, 1)
				assertMetricChartFamily(t, flat, "nagios.perfdata.true.job.execution_state", "Perfdata/true")
				assertMetricValue(t, flat, "nagios.job.execution_duration", metrix.Labels{"nagios_job": "check_disk"}, 2.5)
				assertMetricMeta(t, flat, "nagios.job.execution_duration", "seconds", true)
				if runtime.GOOS != "windows" {
					assertMetricValue(t, flat, "nagios.job.execution_cpu_total", metrix.Labels{"nagios_job": "check_disk"}, 0.5)
					assertMetricValue(t, flat, "nagios.job.execution_max_rss", metrix.Labels{"nagios_job": "check_disk"}, 12345)
					assertMetricMeta(t, flat, "nagios.job.execution_cpu_total", "seconds", true)
					assertMetricMeta(t, flat, "nagios.job.execution_max_rss", "bytes", false)
				} else {
					assertMetricMissing(t, flat, "nagios.job.execution_cpu_total", metrix.Labels{"nagios_job": "check_disk"})
					assertMetricMissing(t, flat, "nagios.job.execution_max_rss", metrix.Labels{"nagios_job": "check_disk"})
				}
				assertMetricValue(t, flat, "nagios.perfdata.true.bytes_used_value", metrix.Labels{"nagios_job": "check_disk", metrix.MeasureSetFieldLabel: "value"}, 30000)
				point, ok := read.MeasureSet("nagios.perfdata.true.bytes_used", metrix.Labels{"nagios_job": "check_disk"})
				require.True(t, ok)
				assert.Equal(t, 30000.0, point.Values[0])

				*now = now.Add(1 * time.Second)
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat = coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_duration", metrix.Labels{"nagios_job": "check_disk"}, 0)
				if runtime.GOOS != "windows" {
					assertMetricValue(t, flat, "nagios.job.execution_cpu_total", metrix.Labels{"nagios_job": "check_disk"}, 0)
					assertMetricValue(t, flat, "nagios.job.execution_max_rss", metrix.Labels{"nagios_job": "check_disk"}, 0)
				} else {
					assertMetricMissing(t, flat, "nagios.job.execution_cpu_total", metrix.Labels{"nagios_job": "check_disk"})
					assertMetricMissing(t, flat, "nagios.job.execution_max_rss", metrix.Labels{"nagios_job": "check_disk"})
				}
				assertMetricValue(t, flat, "nagios.perfdata.true.bytes_used_value", metrix.Labels{"nagios_job": "check_disk", metrix.MeasureSetFieldLabel: "value"}, 30000)
			},
		},
		"uses explicit check name for perfdata namespace": {
			results: []fakeRun{
				{
					result: checkRunResult{
						ServiceState: "OK",
						JobState:     "OK",
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								{Label: "used", Unit: "KB", Value: 30},
							},
						},
					},
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:          "check_service_job",
					CheckName:     "check_service",
					Plugin:        pwshPluginPath,
					Args:          []string{"-NoProfile", "-File", "/opt/netdata/check_service.ps1"},
					CheckInterval: confDuration(5 * time.Minute),
					RetryInterval: confDuration(1 * time.Minute),
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.perfdata.check_service.job.execution_state", metrix.Labels{"nagios_job": "check_service_job", "nagios.perfdata.check_service.job.execution_state": "ok"}, 1)
				assertMetricChartFamily(t, flat, "nagios.perfdata.check_service.job.execution_state", "Perfdata/check_service")
				assertMetricValue(t, flat, "nagios.perfdata.check_service.bytes_used_value", metrix.Labels{"nagios_job": "check_service_job", metrix.MeasureSetFieldLabel: "value"}, 30000)
				assertMetricMissing(t, flat, "nagios.perfdata.pwsh.job.execution_state", metrix.Labels{"nagios_job": "check_service_job", "nagios.perfdata.pwsh.job.execution_state": "ok"})
				assertMetricMissing(t, flat, "nagios.perfdata.pwsh.bytes_used_value", metrix.Labels{"nagios_job": "check_service_job", metrix.MeasureSetFieldLabel: "value"})

				*now = now.Add(1 * time.Second)
			},
		},
		"check period blocked cycles pause job state and zero threshold states": {
			results: []fakeRun{
				{
					result: checkRunResult{
						ServiceState: "OK",
						JobState:     "OK",
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								func() output.PerfDatum {
									low := 0.0
									high := 20.0
									return output.PerfDatum{
										Label: "used",
										Unit:  "KB",
										Value: 30,
										Warn:  &output.ThresholdRange{Low: &low, High: &high},
									}
								}(),
							},
						},
					},
				},
				{
					result: checkRunResult{
						ServiceState: "OK",
						JobState:     "OK",
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								func() output.PerfDatum {
									low := 0.0
									high := 20.0
									return output.PerfDatum{
										Label: "used",
										Unit:  "KB",
										Value: 10,
										Warn:  &output.ThresholdRange{Low: &low, High: &high},
									}
								}(),
							},
						},
					},
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:          "period_job",
					Plugin:        truePluginPath,
					CheckInterval: confDuration(1 * time.Hour),
					RetryInterval: confDuration(1 * time.Minute),
					CheckPeriod:   "business",
				},
				TimePeriods: []timeperiod.Config{
					{
						Name: "business",
						Rules: []timeperiod.RuleConfig{
							{
								Type:   "weekly",
								Days:   []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"},
								Ranges: []string{"09:00-18:00"},
							},
						},
					},
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.perfdata.true.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.bytes_used_value", metrix.Labels{"nagios_job": "period_job", metrix.MeasureSetFieldLabel: "value"}, 30000)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "period_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateWarning,
				}, 1)

				raw := coll.MetricStore().Read()
				thresholdMetric := "nagios.perfdata.true.bytes_used_threshold_state"
				thresholdLabels := metrix.Labels{"nagios_job": "period_job"}
				point, ok := raw.StateSet(thresholdMetric, thresholdLabels)
				require.True(t, ok)
				assert.True(t, point.States[perfThresholdStateWarning])
				alertThresholdMetric := "nagios.job.perfdata.threshold_state"
				alertThresholdLabels := metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used"}
				alertPoint, ok := raw.StateSet(alertThresholdMetric, alertThresholdLabels)
				require.True(t, ok)
				assert.True(t, alertPoint.States[perfThresholdStateWarning])

				*now = time.Date(2026, 3, 23, 20, 0, 0, 0, time.UTC)
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat = coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.job.execution_state": "paused"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.job.execution_state": "retry"}, 0)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.perfdata.true.job.execution_state": "paused"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.perfdata.true.job.execution_state": "retry"}, 0)
				assertMetricValue(t, flat, "nagios.perfdata.true.bytes_used_value", metrix.Labels{"nagios_job": "period_job", metrix.MeasureSetFieldLabel: "value"}, 30000)
				assertMetricValue(t, flat, thresholdMetric, metrix.Labels{"nagios_job": "period_job", thresholdMetric: perfThresholdStateWarning}, 0)
				assertMetricValue(t, flat, thresholdMetric, metrix.Labels{"nagios_job": "period_job", thresholdMetric: perfThresholdStateOK}, 0)
				assertMetricValue(t, flat, thresholdMetric, metrix.Labels{"nagios_job": "period_job", thresholdMetric: perfThresholdStateCritical}, 0)
				assertMetricValue(t, flat, thresholdMetric, metrix.Labels{"nagios_job": "period_job", thresholdMetric: perfThresholdStateNone}, 0)
				assertMetricValue(t, flat, alertThresholdMetric, metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used", alertThresholdMetric: perfThresholdStateWarning}, 0)
				assertMetricValue(t, flat, alertThresholdMetric, metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used", alertThresholdMetric: perfThresholdStateOK}, 0)
				assertMetricValue(t, flat, alertThresholdMetric, metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used", alertThresholdMetric: perfThresholdStateCritical}, 0)
				assertMetricValue(t, flat, alertThresholdMetric, metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used", alertThresholdMetric: perfThresholdStateNone}, 0)
				assertMetricValue(t, flat, alertThresholdMetric, metrix.Labels{"nagios_job": "period_job", perfdataValueLabelKey: "bytes_used", alertThresholdMetric: perfThresholdStateRetry}, 0)

				raw = coll.MetricStore().Read()
				point, ok = raw.StateSet(thresholdMetric, thresholdLabels)
				require.True(t, ok)
				assert.False(t, point.States[perfThresholdStateNone])
				assert.False(t, point.States[perfThresholdStateOK])
				assert.False(t, point.States[perfThresholdStateWarning])
				assert.False(t, point.States[perfThresholdStateCritical])
				alertPoint, ok = raw.StateSet(alertThresholdMetric, alertThresholdLabels)
				require.True(t, ok)
				assert.False(t, alertPoint.States[perfThresholdStateNone])
				assert.False(t, alertPoint.States[perfThresholdStateOK])
				assert.False(t, alertPoint.States[perfThresholdStateWarning])
				assert.False(t, alertPoint.States[perfThresholdStateCritical])
				assert.False(t, alertPoint.States[perfThresholdStateRetry])

				*now = time.Date(2026, 3, 24, 9, 0, 0, 0, time.UTC)
				runCollectCycle(t, coll)
				assert.Equal(t, 2, runner.calls)

				flat = coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "period_job", "nagios.perfdata.true.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.bytes_used_value", metrix.Labels{"nagios_job": "period_job", metrix.MeasureSetFieldLabel: "value"}, 10000)

				raw = coll.MetricStore().Read()
				point, ok = raw.StateSet(thresholdMetric, thresholdLabels)
				require.True(t, ok)
				assert.False(t, point.States[perfThresholdStateNone])
				assert.True(t, point.States[perfThresholdStateOK])
				assert.False(t, point.States[perfThresholdStateWarning])
				assert.False(t, point.States[perfThresholdStateCritical])
				alertPoint, ok = raw.StateSet(alertThresholdMetric, alertThresholdLabels)
				require.True(t, ok)
				assert.False(t, alertPoint.States[perfThresholdStateNone])
				assert.True(t, alertPoint.States[perfThresholdStateOK])
				assert.False(t, alertPoint.States[perfThresholdStateWarning])
				assert.False(t, alertPoint.States[perfThresholdStateCritical])
			},
		},
		"uses retry interval for retry state": {
			results: []fakeRun{
				{
					result: checkRunResult{
						ServiceState: "WARNING",
						JobState:     "WARNING",
						ExitCode:     1,
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								func() output.PerfDatum {
									low := 0.0
									high := 20.0
									return output.PerfDatum{
										Label: "used",
										Unit:  "KB",
										Value: 30,
										Warn:  &output.ThresholdRange{Low: &low, High: &high},
									}
								}(),
							},
						},
					},
					err: errors.New("plugin returned warning"),
				},
				{
					result: checkRunResult{
						ServiceState: "OK",
						JobState:     "OK",
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								func() output.PerfDatum {
									low := 0.0
									high := 20.0
									return output.PerfDatum{
										Label: "used",
										Unit:  "KB",
										Value: 10,
										Warn:  &output.ThresholdRange{Low: &low, High: &high},
									}
								}(),
							},
						},
					},
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:             "retry_job",
					Plugin:           truePluginPath,
					CheckInterval:    confDuration(5 * time.Minute),
					RetryInterval:    confDuration(10 * time.Second),
					MaxCheckAttempts: 3,
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)
				assert.Equal(t, 2, coll.state.currentAttempt())
				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.job.execution_state": "warning"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.job.execution_state": "retry"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.perfdata.true.job.execution_state": "warning"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.perfdata.true.job.execution_state": "retry"}, 1)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateWarning,
				}, 1)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateRetry,
				}, 1)

				*now = now.Add(9 * time.Second)
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				*now = now.Add(2 * time.Second)
				runCollectCycle(t, coll)
				assert.Equal(t, 2, runner.calls)
				assert.Equal(t, 1, coll.state.currentAttempt())
				flat = coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.job.execution_state": "ok"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "retry_job", "nagios.job.execution_state": "retry"}, 0)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateOK,
				}, 1)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateRetry,
				}, 0)
			},
		},
		"period block suppresses public retry state": {
			results: []fakeRun{
				{
					result: checkRunResult{
						ServiceState: "WARNING",
						JobState:     "WARNING",
						ExitCode:     1,
						Parsed: output.ParsedOutput{
							Perfdata: []output.PerfDatum{
								func() output.PerfDatum {
									low := 0.0
									high := 20.0
									return output.PerfDatum{
										Label: "used",
										Unit:  "KB",
										Value: 30,
										Warn:  &output.ThresholdRange{Low: &low, High: &high},
									}
								}(),
							},
						},
					},
					err: errors.New("plugin returned warning"),
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:             "paused_retry_job",
					Plugin:           truePluginPath,
					CheckInterval:    confDuration(1 * time.Hour),
					RetryInterval:    confDuration(10 * time.Second),
					MaxCheckAttempts: 3,
					CheckPeriod:      "business",
				},
				TimePeriods: []timeperiod.Config{
					{
						Name: "business",
						Rules: []timeperiod.RuleConfig{
							{
								Type:   "weekly",
								Days:   []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"},
								Ranges: []string{"09:00-18:00"},
							},
						},
					},
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.job.execution_state": "warning"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.job.execution_state": "retry"}, 1)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "paused_retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateWarning,
				}, 1)
				assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":                          "paused_retry_job",
					perfdataValueLabelKey:                 "bytes_used",
					"nagios.job.perfdata.threshold_state": perfThresholdStateRetry,
				}, 1)

				*now = time.Date(2026, 3, 23, 20, 0, 0, 0, time.UTC)
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)

				flat = coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.job.execution_state": "paused"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.job.execution_state": "retry"}, 0)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.perfdata.true.job.execution_state": "paused"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "paused_retry_job", "nagios.perfdata.true.job.execution_state": "retry"}, 0)
				for _, state := range perfThresholdAlertStateNames {
					assertMetricValue(t, flat, "nagios.job.perfdata.threshold_state", metrix.Labels{
						"nagios_job":                          "paused_retry_job",
						perfdataValueLabelKey:                 "bytes_used",
						"nagios.job.perfdata.threshold_state": state,
					}, 0)
				}

				raw := coll.MetricStore().Read()
				alertPoint, ok := raw.StateSet("nagios.job.perfdata.threshold_state", metrix.Labels{
					"nagios_job":          "paused_retry_job",
					perfdataValueLabelKey: "bytes_used",
				})
				require.True(t, ok)
				for _, state := range perfThresholdAlertStateNames {
					assert.False(t, alertPoint.States[state])
				}
			},
		},
		"timeout is exposed publicly but macros keep Nagios unknown": {
			results: []fakeRun{
				{
					result: checkRunResult{ServiceState: nagiosStateUnknown, JobState: jobStateTimeout, ExitCode: -1},
					err:    errNagiosCheckTimeout,
				},
				{
					result: checkRunResult{ServiceState: "OK", JobState: "OK"},
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:             "timeout_job",
					Plugin:           truePluginPath,
					CheckInterval:    confDuration(5 * time.Minute),
					RetryInterval:    confDuration(10 * time.Second),
					MaxCheckAttempts: 3,
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, now *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				assert.Equal(t, 1, runner.calls)
				assert.Equal(t, nagiosStateUnknown, coll.state.currentServiceState())
				assert.Equal(t, jobStateTimeout, coll.state.currentJobState())

				flat := coll.MetricStore().Read(metrix.ReadFlatten())
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "timeout_job", "nagios.job.execution_state": "timeout"}, 1)
				assertMetricValue(t, flat, "nagios.job.execution_state", metrix.Labels{"nagios_job": "timeout_job", "nagios.job.execution_state": "retry"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "timeout_job", "nagios.perfdata.true.job.execution_state": "timeout"}, 1)
				assertMetricValue(t, flat, "nagios.perfdata.true.job.execution_state", metrix.Labels{"nagios_job": "timeout_job", "nagios.perfdata.true.job.execution_state": "retry"}, 1)

				*now = now.Add(11 * time.Second)
				runCollectCycle(t, coll)
				require.Len(t, runner.reqs, 2)
				assert.Equal(t, nagiosStateUnknown, runner.reqs[1].MacroState.ServiceState)
				assert.Equal(t, 2, runner.reqs[1].MacroState.ServiceAttempt)
			},
		},
		"infrastructure failures return error and keep state unchanged": {
			results: []fakeRun{
				{
					result: checkRunResult{ServiceState: "UNKNOWN", JobState: "UNKNOWN", ExitCode: -1},
					err:    errors.New("spawn failed"),
				},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:   "infra_fail",
					Plugin: truePluginPath,
				},
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, _ *time.Time) {
				t.Helper()
				cc := mustCycleController(t, coll.MetricStore())
				cc.BeginCycle()
				err := coll.Collect(context.Background())
				cc.AbortCycle()
				require.Error(t, err)
				assert.Equal(t, 1, runner.calls)
				assert.Equal(t, nagiosStateUnknown, coll.state.currentServiceState())
				assert.Equal(t, nagiosStateUnknown, coll.state.currentJobState())
			},
		},
		"passes virtual node to runner": {
			results: []fakeRun{
				{result: checkRunResult{ServiceState: "OK", JobState: "OK"}},
			},
			config: Config{
				UpdateEvery: 1,
				JobConfig: JobConfig{
					Name:   "with_vnode",
					Plugin: truePluginPath,
				},
			},
			setup: func(coll *Collector, _ *fakeRunner, _ *time.Time) {
				coll.vnode = vnodes.VirtualNode{
					Hostname: "node-a",
					Labels: map[string]string{
						"_address": "203.0.113.10",
						"_alias":   "node-a-alias",
						"_DC":      "east",
						"region":   "lab",
					},
				}
			},
			run: func(t *testing.T, coll *Collector, runner *fakeRunner, _ *time.Time) {
				t.Helper()
				runCollectCycle(t, coll)
				require.Len(t, runner.reqs, 1)
				req := runner.reqs[0]
				assert.Equal(t, "node-a", req.Vnode.Hostname)
				assert.Equal(t, "203.0.113.10", req.Vnode.Labels["_address"])
				assert.Equal(t, "lab", req.Vnode.Labels["region"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
			runner := &fakeRunner{results: tc.results}
			coll := newTestCollector()
			coll.runner = runner
			coll.now = func() time.Time { return now }
			coll.Config = tc.config
			if tc.setup != nil {
				tc.setup(coll, runner, &now)
			}
			require.NoError(t, coll.Init(context.Background()))
			tc.run(t, coll, runner, &now)
		})
	}
}

func TestBuildMacroSet(t *testing.T) {
	now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		job    JobConfig
		vnode  vnodeInfo
		state  macroState
		assert func(*testing.T, macroSet)
	}{
		"includes vnode and service macros": {
			job: JobConfig{
				Name:      "http_check",
				Plugin:    "/usr/lib/nagios/plugins/check_http",
				Args:      []string{"-H", "$HOSTADDRESS$", "-p", "$ARG1$", "-w", "$ARG2$"},
				ArgValues: []string{"8080", "5"},
				CustomVars: map[string]string{
					"ENDPOINT": "/health",
				},
				Vnode: "fallback-host",
			},
			vnode: vnodeInfo{
				Hostname: "web1",
				Labels: map[string]string{
					"_address":    "192.0.2.10",
					"_alias":      "web-node",
					"_DATACENTER": "us-east-1",
					"role":        "frontend",
				},
			},
			state: macroState{
				ServiceState:       "OK",
				ServiceAttempt:     2,
				ServiceMaxAttempts: 5,
			},
			assert: func(t *testing.T, s macroSet) {
				t.Helper()
				assert.Equal(t, "192.0.2.10", s.Env["NAGIOS_HOSTADDRESS"])
				assert.Equal(t, "web-node", s.Env["NAGIOS_HOSTALIAS"])
				assert.Equal(t, "/health", s.Env["NAGIOS__SERVICEENDPOINT"])
				assert.Equal(t, "us-east-1", s.Env["NAGIOS__HOSTDATACENTER"])
				assert.Equal(t, "frontend", s.Env["NAGIOS__HOSTLABEL_ROLE"])
				assert.Equal(t, "8080", s.Env["NAGIOS_ARG1"])
				assert.Equal(t, "2", s.Env["NAGIOS_SERVICEATTEMPT"])
				assert.Equal(t, nagiosHostStateUp, s.Env["NAGIOS_HOSTSTATE"])
				assert.Equal(t, nagiosHostStateUpID, s.Env["NAGIOS_HOSTSTATEID"])
				assert.Equal(t, "192.0.2.10", s.CommandArgs[1])
				assert.Equal(t, "8080", s.CommandArgs[3])
			},
		},
		"falls back to job vnode when runtime vnode is empty": {
			job: JobConfig{
				Name:   "fallback",
				Plugin: "/bin/true",
				Args:   []string{"$HOSTNAME$"},
				Vnode:  "fallback-host",
			},
			vnode: vnodeInfo{
				Labels: map[string]string{},
			},
			state: macroState{ServiceState: "OK"},
			assert: func(t *testing.T, s macroSet) {
				t.Helper()
				assert.Equal(t, "fallback-host", s.Env["NAGIOS_HOSTNAME"])
				assert.Equal(t, "fallback-host", s.CommandArgs[0])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := buildMacroSet(tc.job, tc.vnode, tc.state, now)
			tc.assert(t, got)
		})
	}
}

func TestReplaceMacro(t *testing.T) {
	tests := map[string]struct {
		value string
		env   map[string]string
		want  string
	}{
		"expands nested macros deterministically": {
			value: "$ARG1$",
			env: map[string]string{
				"NAGIOS_ARG1":        "$HOSTADDRESS$:$ARG2$",
				"NAGIOS_ARG2":        "8080",
				"NAGIOS_HOSTADDRESS": "192.0.2.10",
			},
			want: "192.0.2.10:8080",
		},
		"keeps unknown macros unchanged": {
			value: "$UNKNOWN$:$ARG1$",
			env: map[string]string{
				"NAGIOS_ARG1": "value",
			},
			want: "$UNKNOWN$:value",
		},
		"stops recursive cycles deterministically": {
			value: "$ARG1$",
			env: map[string]string{
				"NAGIOS_ARG1": "$ARG2$",
				"NAGIOS_ARG2": "$ARG1$",
			},
			want: "$ARG1$",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, replaceMacro(tc.value, tc.env))
		})
	}
}

func TestBuildRunEnv(t *testing.T) {
	t.Setenv("NAGIOS_TEST_LEAK", "secret")
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")
	t.Setenv("TZ", "UTC")

	tests := map[string]struct {
		workingDir string
		jobEnv     map[string]string
		macroEnv   map[string]string
		assert     func(*testing.T, map[string]string)
	}{
		"uses explicit baseline and does not leak ambient env": {
			jobEnv:   map[string]string{},
			macroEnv: map[string]string{},
			assert: func(t *testing.T, env map[string]string) {
				t.Helper()
				assert.NotContains(t, env, "NAGIOS_TEST_LEAK")
				assert.Equal(t, "UTC", env["TZ"])
				assert.Equal(t, "/usr/local/bin:/usr/bin", env["PATH"])
				if runtime.GOOS != "windows" {
					assert.Equal(t, "C", env["LC_ALL"])
					assert.Equal(t, "/bin/sh", env["SHELL"])
				}
			},
		},
		"uses actual current directory instead of inherited parent PWD": {
			jobEnv:   map[string]string{},
			macroEnv: map[string]string{},
			assert: func(t *testing.T, env map[string]string) {
				t.Helper()
				if runtime.GOOS == "windows" {
					return
				}
				cwd, err := os.Getwd()
				require.NoError(t, err)
				assert.Equal(t, cwd, env["PWD"])
				assert.NotEqual(t, "/parent/pwd", env["PWD"])
			},
		},
		"working directory overrides PWD": {
			workingDir: "/tmp/checks",
			jobEnv:     map[string]string{},
			macroEnv:   map[string]string{},
			assert: func(t *testing.T, env map[string]string) {
				t.Helper()
				if runtime.GOOS == "windows" {
					return
				}
				assert.Equal(t, "/tmp/checks", env["PWD"])
			},
		},
		"job environment overrides baseline and macros override job environment": {
			jobEnv: map[string]string{
				"PATH":        "/custom/bin",
				"NAGIOS_ARG1": "user-value",
				"TARGET":      "$ARG1$",
			},
			macroEnv: map[string]string{
				"NAGIOS_ARG1": "macro-value",
			},
			assert: func(t *testing.T, env map[string]string) {
				t.Helper()
				assert.Equal(t, "/custom/bin", env["PATH"])
				assert.Equal(t, "macro-value", env["NAGIOS_ARG1"])
				assert.Equal(t, "macro-value", env["TARGET"])
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if runtime.GOOS != "windows" {
				t.Setenv("PWD", "/parent/pwd")
			}
			env := envSliceToMap(buildRunEnv(tc.workingDir, tc.jobEnv, tc.macroEnv))
			tc.assert(t, env)
		})
	}
}

func TestSystemCheckRunner_EnvironmentContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh scripts")
	}

	t.Setenv("NAGIOS_TEST_LEAK", "secret")
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")
	t.Setenv("TZ", "UTC")

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "check_env.sh")
	writeExecutable(t, scriptPath, `#!/bin/sh
set -eu
printf '%s\n' 'OK - env contract | value=1;;;;'
printf 'EXPLICIT=%s\n' "${EXPLICIT:-}"
printf 'EXPANDED=%s\n' "${EXPANDED:-}"
printf 'HOSTADDRESS=%s\n' "${NAGIOS_HOSTADDRESS:-}"
printf 'ARG1=%s\n' "${NAGIOS_ARG1:-}"
printf 'LEAK=%s\n' "${NAGIOS_TEST_LEAK:-}"
printf 'LC_ALL=%s\n' "${LC_ALL:-}"
`)

	job := JobConfig{
		Name:          "env_contract",
		Plugin:        scriptPath,
		ArgValues:     []string{"8080"},
		Environment:   map[string]string{"EXPLICIT": "from-job", "EXPANDED": "$ARG1$"},
		Timeout:       confDuration(5 * time.Second),
		CheckInterval: confDuration(5 * time.Minute),
		RetryInterval: confDuration(1 * time.Minute),
	}

	result, err := systemCheckRunner{}.Run(context.Background(), checkRunRequest{
		Job: job,
		Vnode: vnodeInfo{
			Hostname: "node-a",
			Labels: map[string]string{
				"_address": "192.0.2.10",
			},
		},
		MacroState: macroState{ServiceState: nagiosStateOK},
		Now:        time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	assert.Equal(t, nagiosStateOK, result.ServiceState)
	assert.Equal(t, nagiosStateOK, result.JobState)
	assert.Equal(t, "OK - env contract", result.Parsed.StatusLine())
	assert.Contains(t, result.Parsed.LongOutput(), "EXPLICIT=from-job")
	assert.Contains(t, result.Parsed.LongOutput(), "EXPANDED=8080")
	assert.Contains(t, result.Parsed.LongOutput(), "HOSTADDRESS=192.0.2.10")
	assert.Contains(t, result.Parsed.LongOutput(), "ARG1=8080")
	assert.Contains(t, result.Parsed.LongOutput(), "LEAK=")
	assert.NotContains(t, result.Parsed.LongOutput(), "LEAK=secret")
	assert.Contains(t, result.Parsed.LongOutput(), "LC_ALL=C")
}

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

type fakeRun struct {
	result checkRunResult
	err    error
}

type fakeRunner struct {
	results []fakeRun
	reqs    []checkRunRequest
	calls   int
}

func (f *fakeRunner) Run(_ context.Context, req checkRunRequest) (checkRunResult, error) {
	f.reqs = append(f.reqs, req)
	if f.calls >= len(f.results) {
		f.calls++
		return checkRunResult{}, nil
	}
	run := f.results[f.calls]
	f.calls++
	return run.result, run.err
}

func runCollectCycle(t *testing.T, coll *Collector) {
	t.Helper()
	cc := mustCycleController(t, coll.MetricStore())
	cc.BeginCycle()
	if err := coll.Collect(context.Background()); err != nil {
		cc.AbortCycle()
		require.NoError(t, err)
	}
	cc.CommitCycleSuccess()
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func assertMetricValue(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.True(t, ok, "missing metric %s labels=%v", name, labels)
	assert.InDelta(t, want, got, 1e-9, "metric mismatch %s labels=%v", name, labels)
}

func assertMetricMissing(t *testing.T, r metrix.Reader, name string, labels metrix.Labels) {
	t.Helper()
	_, ok := r.Value(name, labels)
	assert.False(t, ok, "unexpected metric %s labels=%v", name, labels)
}

func writeTestPluginFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	mode := os.FileMode(0o644)
	content := "placeholder\n"
	if runtime.GOOS != "windows" {
		mode = 0o755
		content = "#!/bin/sh\nexit 0\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	return path
}

func newTestCollector() *Collector {
	coll := New()
	coll.validatePlugin = func(path string) (string, error) { return path, nil }
	return coll
}

func confDuration(d time.Duration) confopt.Duration { return confopt.Duration(d) }

func findChartDimensionByContext(specYAML *charttpl.Spec, context string) (charttpl.Dimension, bool) {
	for _, group := range specYAML.Groups {
		if dim, ok := findChartDimensionInGroup(group, context); ok {
			return dim, true
		}
	}
	return charttpl.Dimension{}, false
}

func findChartDimensionInGroup(group charttpl.Group, context string) (charttpl.Dimension, bool) {
	for _, chart := range group.Charts {
		if chart.Context != context || len(chart.Dimensions) == 0 {
			continue
		}
		return chart.Dimensions[0], true
	}
	for _, child := range group.Groups {
		if dim, ok := findChartDimensionInGroup(child, context); ok {
			return dim, true
		}
	}
	return charttpl.Dimension{}, false
}
