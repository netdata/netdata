// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"runtime"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

func (c *Collector) collect(ctx context.Context) error {
	execMetrics, err := c.collectIfDue(ctx)
	if err != nil {
		return err
	}
	c.emitMetrics(execMetrics)
	return nil
}

func (c *Collector) collectIfDue(ctx context.Context) (executionMetrics, error) {
	now := c.now()
	if !c.state.due(now) {
		return executionMetrics{}, nil
	}

	if c.skipDisallowedPeriod(now) {
		return executionMetrics{}, nil
	}

	res, err := c.executeDueCheck(ctx, now)
	if err != nil {
		return executionMetrics{}, err
	}

	c.completeDueCheck(now, res)
	return executionMetricsFromResult(res), nil
}

func (c *Collector) skipDisallowedPeriod(now time.Time) bool {
	if c.job.period == nil || c.job.period.Allows(now) {
		return false
	}
	c.state.recordPeriodBlocked()
	c.state.scheduleNextAllowed(now, c.job.config.CheckInterval.Duration(), c.job.period)
	return true
}

func (c *Collector) executeDueCheck(ctx context.Context, now time.Time) (checkRunResult, error) {
	res, err := c.runner.Run(ctx, checkRunRequest{
		Job:        c.job.config,
		Vnode:      vnodeInfoFromVirtualNode(c.VirtualNode(), c.job.config.Vnode),
		MacroState: c.state.macroState(),
		Now:        now,
		Log:        c.Logger,
	})
	if err != nil {
		if runErr := classifyRunError(ctx, res.ExitCode, err); runErr != nil {
			return checkRunResult{}, runErr
		}
	}
	return res, nil
}

func (c *Collector) completeDueCheck(now time.Time, res checkRunResult) {
	c.state.completeRun(now, res.ServiceState, res.JobState, c.router.route(c.job.config.Plugin, res.Parsed.Perfdata), c.job.config)
}

func (c *Collector) emitMetrics(execMetrics executionMetrics) {
	sm := c.store.Write().SnapshotMeter("nagios")

	jobName := c.job.config.Name
	if jobName == "" {
		jobName = c.Config.JobConfig.Name
	}

	jobLbl := sm.LabelSet(metrix.Label{Key: "nagios_job", Value: jobName})
	jobMeter := sm.WithLabelSet(jobLbl)
	jobStatePoint := projectJobExecutionState(c.state.currentJobState(), c.state.isRetrying())

	jobMeter.StateSet(
		"job.execution_state",
		metrix.WithStateSetMode(metrix.ModeBitSet),
		metrix.WithStateSetStates(jobExecutionStateNames...),
		metrix.WithUnit("state"),
	).ObserveStateSet(jobStatePoint)

	scriptName := perfSourceFromPlugin(c.job.config.Plugin)
	jobMeter.StateSet(
		"perfdata."+scriptName+".job.execution_state",
		metrix.WithStateSetMode(metrix.ModeBitSet),
		metrix.WithStateSetStates(jobExecutionStateNames...),
		metrix.WithChartFamily(perfdataFamily(scriptName)),
		metrix.WithChartPriority(chartengine.Priority-10),
		metrix.WithUnit("state"),
	).ObserveStateSet(jobStatePoint)

	jobMeter.Gauge(
		"job.execution_duration",
		metrix.WithUnit("seconds"),
		metrix.WithFloat(true),
	).Observe(execMetrics.durationSeconds)

	if runtime.GOOS != "windows" {
		jobMeter.Gauge(
			"job.execution_cpu_total",
			metrix.WithUnit("seconds"),
			metrix.WithFloat(true),
		).Observe(execMetrics.cpuTotalSeconds)

		jobMeter.Gauge(
			"job.execution_max_rss",
			metrix.WithUnit("bytes"),
		).Observe(execMetrics.maxRSSBytes)
	}

	for _, measureSet := range c.state.perfValueSets() {
		fields := perfMeasureSetValues(measureSet.value)
		if measureSet.counter {
			jobMeter.MeasureSetCounter(
				measureSet.name,
				metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
				metrix.WithChartFamily(perfdataFamily(measureSet.scriptName)),
				metrix.WithUnit(measureSet.unit),
				metrix.WithFloat(true),
			).ObserveTotalFields(fields)
		} else {
			jobMeter.MeasureSetGauge(
				measureSet.name,
				metrix.WithMeasureSetFields(perfMeasureSetFieldSpecs()...),
				metrix.WithChartFamily(perfdataFamily(measureSet.scriptName)),
				metrix.WithUnit(measureSet.unit),
				metrix.WithFloat(true),
			).ObserveFields(fields)
		}
	}

	for _, thresholdState := range c.state.perfThresholdStates() {
		inst := jobMeter.StateSet(
			thresholdState.name,
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates(perfThresholdStateNames...),
			metrix.WithChartFamily(perfdataFamily(thresholdState.scriptName)),
			metrix.WithUnit("state"),
		)
		if thresholdState.state == "" {
			inst.ObserveStateSet(perfThresholdStatePoint(""))
		} else {
			inst.Enable(thresholdState.state)
		}

		jobMeter.WithLabels(metrix.Label{
			Key:   perfdataValueLabelKey,
			Value: thresholdState.perfdataValue,
		}).StateSet(
			jobPerfdataThresholdMetricName,
			metrix.WithStateSetMode(metrix.ModeBitSet),
			metrix.WithStateSetStates(perfThresholdAlertStateNames...),
			metrix.WithUnit("state"),
		).ObserveStateSet(projectPerfThresholdAlertState(thresholdState.state, c.state.isRetrying()))
	}
}

func perfdataFamily(scriptName string) string {
	return "Perfdata/" + scriptName
}
