// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const functionsRuntimeMetricPrefix = "netdata.go.plugin.framework.functions.manager"

type managerRuntimeMetrics struct {
	invocationsActive         metrix.StatefulGauge
	invocationsAwaitingResult metrix.StatefulGauge
	schedulerPending          metrix.StatefulGauge

	functionCallsTotal  metrix.StatefulCounter
	queueFullTotal      metrix.StatefulCounter
	cancelFallbackTotal metrix.StatefulCounter
	lateTerminalDropped metrix.StatefulCounter
	duplicateUIDIgnored metrix.StatefulCounter
}

func newManagerRuntimeMetrics(store metrix.RuntimeStore) *managerRuntimeMetrics {
	if store == nil {
		return nil
	}

	meter := store.Write().StatefulMeter(functionsRuntimeMetricPrefix)
	metrics := &managerRuntimeMetrics{
		invocationsActive: metrix.SeededGauge(meter,
			"invocations_active",
			metrix.WithDescription("Current number of active function invocations tracked by UID"),
			metrix.WithChartFamily("Framework/Functions/Invocations"),
			metrix.WithUnit("invocations"),
		),
		invocationsAwaitingResult: metrix.SeededGauge(meter,
			"invocations_awaiting_result",
			metrix.WithDescription("Current number of active invocations waiting for terminal response"),
			metrix.WithChartFamily("Framework/Functions/Invocations"),
			metrix.WithUnit("invocations"),
		),
		schedulerPending: metrix.SeededGauge(meter,
			"scheduler_pending",
			metrix.WithDescription("Current number of invocations pending in scheduler"),
			metrix.WithChartFamily("Framework/Functions/Scheduler"),
			metrix.WithUnit("invocations"),
		),
		functionCallsTotal: metrix.SeededCounter(meter,
			"calls_total",
			metrix.WithDescription("Total number of parsed function call requests"),
			metrix.WithChartFamily("Framework/Functions/Calls"),
			metrix.WithUnit("calls"),
		),
		queueFullTotal: metrix.SeededCounter(meter,
			"queue_full_total",
			metrix.WithDescription("Total number of function requests rejected due to queue full"),
			metrix.WithChartFamily("Framework/Functions/Failures"),
			metrix.WithUnit("requests"),
		),
		cancelFallbackTotal: metrix.SeededCounter(meter,
			"cancel_fallback_total",
			metrix.WithDescription("Total number of function requests finalized by cancel fallback timer"),
			metrix.WithChartFamily("Framework/Functions/Cancellation"),
			metrix.WithUnit("requests"),
		),
		lateTerminalDropped: metrix.SeededCounter(meter,
			"late_terminal_dropped_total",
			metrix.WithDescription("Total number of late terminal responses dropped by tombstone guard"),
			metrix.WithChartFamily("Framework/Functions/Finalization"),
			metrix.WithUnit("responses"),
		),
		duplicateUIDIgnored: metrix.SeededCounter(meter,
			"duplicate_uid_ignored_total",
			metrix.WithDescription("Total number of duplicate transaction IDs ignored at admission"),
			metrix.WithChartFamily("Framework/Functions/Admission"),
			metrix.WithUnit("requests"),
		),
	}

	return metrics
}

func (m *Manager) observeInvocationsLocked() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}

	active := len(m.invState)
	awaiting := 0
	for _, rec := range m.invState {
		if rec != nil && rec.state == stateAwaitingResult {
			awaiting++
		}
	}

	m.runtimeMetrics.invocationsActive.Set(float64(active))
	m.runtimeMetrics.invocationsAwaitingResult.Set(float64(awaiting))
}

func (m *Manager) observeSchedulerPending() {
	if m == nil || m.runtimeMetrics == nil || m.scheduler == nil {
		return
	}
	m.runtimeMetrics.schedulerPending.Set(float64(m.scheduler.pendingCount()))
}

func (m *Manager) observeQueueFull() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}
	m.runtimeMetrics.queueFullTotal.Add(1)
}

func (m *Manager) observeFunctionCall() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}
	m.runtimeMetrics.functionCallsTotal.Add(1)
}

func (m *Manager) observeCancelFallback() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}
	m.runtimeMetrics.cancelFallbackTotal.Add(1)
}

func (m *Manager) observeLateTerminalDropped() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}
	m.runtimeMetrics.lateTerminalDropped.Add(1)
}

func (m *Manager) observeDuplicateUIDIgnored() {
	if m == nil || m.runtimeMetrics == nil {
		return
	}
	m.runtimeMetrics.duplicateUIDIgnored.Add(1)
}
