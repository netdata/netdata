// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

const (
	executorRuntimeMetricPrefix  = "netdata.go.plugin.agent.jobmgr.executor"
	executorRuntimeComponentName = "jobmgr.executor"
	executorRuntimeProducerName  = "jobmgr.executor.ages"
)

// executorRuntimeMetrics exposes the executor's lane and pool state. Gauges
// and counters are written from the run loop (and pool workers for effect
// accounting); the age gauges are computed by a runtimecomp producer tick
// from atomics the loop maintains, so the ticker never reads loop-owned
// state.
type executorRuntimeMetrics struct {
	busyKeys       metrix.StatefulGauge
	waitParkedKeys metrix.StatefulGauge
	parkedEvents   metrix.StatefulGauge
	poolInflight   metrix.StatefulGauge
	poolQueued     metrix.StatefulGauge

	oldestParkedAge metrix.StatefulGauge
	oldestWaitAge   metrix.StatefulGauge

	effectsStarted       metrix.StatefulCounter
	effectPanics         metrix.StatefulCounter
	parkedTotal          metrix.StatefulCounter
	shutdownRejected     metrix.StatefulCounter
	leakedEffects        metrix.StatefulCounter
	wedgedKeys           metrix.StatefulCounter
	staleCommits         metrix.StatefulCounter
	effectBusySeconds    metrix.StatefulCounter
	suppressedLateOutput metrix.StatefulCounter
	barrierWaitSeconds   metrix.StatefulCounter

	// Unix nanos of the oldest currently parked event / wait-parked key;
	// 0 when none. Written on the run loop, read by the producer tick.
	oldestParkedSince atomic.Int64
	oldestWaitSince   atomic.Int64
}

func newExecutorRuntimeMetrics(store metrix.RuntimeStore) *executorRuntimeMetrics {
	if store == nil {
		return nil
	}

	meter := store.Write().StatefulMeter(executorRuntimeMetricPrefix)
	return &executorRuntimeMetrics{
		busyKeys: metrix.SeededGauge(meter, "busy_keys",
			metrix.WithDescription("Keys with a blocking phase in flight"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("keys"),
		),
		waitParkedKeys: metrix.SeededGauge(meter, "wait_parked_keys",
			metrix.WithDescription("Keys parked awaiting an enable/disable decision"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("keys"),
		),
		parkedEvents: metrix.SeededGauge(meter, "parked_events",
			metrix.WithDescription("Events parked behind busy or wait-parked keys"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("events"),
		),
		poolInflight: metrix.SeededGauge(meter, "pool_inflight",
			metrix.WithDescription("Blocking phases in flight: executing on the effect pool or awaiting a slot"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("effects"),
		),
		poolQueued: metrix.SeededGauge(meter, "pool_queued",
			metrix.WithDescription("Dispatched blocking phases awaiting a pool slot"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("effects"),
		),
		oldestParkedAge: metrix.SeededGauge(meter, "oldest_parked_age",
			metrix.WithDescription("Age of the oldest parked event"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("seconds"),
		),
		oldestWaitAge: metrix.SeededGauge(meter, "oldest_wait_age",
			metrix.WithDescription("Age of the oldest wait-parked key"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("seconds"),
		),
		effectsStarted: metrix.SeededCounter(meter, "effects_started",
			metrix.WithDescription("Blocking phases dispatched to the effect pool"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("effects"),
		),
		effectPanics: metrix.SeededCounter(meter, "effect_panics",
			metrix.WithDescription("Effect panics converted to failed commands"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("panics"),
		),
		parkedTotal: metrix.SeededCounter(meter, "parked_events_admitted",
			metrix.WithDescription("Events that parked behind a busy or wait-parked key"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("events"),
		),
		shutdownRejected: metrix.SeededCounter(meter, "shutdown_rejected",
			metrix.WithDescription("Parked dyncfg commands answered 503 at shutdown"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("commands"),
		),
		leakedEffects: metrix.SeededCounter(meter, "leaked_effects",
			metrix.WithDescription("Effects abandoned at their deadline with the module call still running"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("effects"),
		),
		wedgedKeys: metrix.SeededCounter(meter, "wedged_keys",
			metrix.WithDescription("Keys held busy by an abandoned effect awaiting its late return"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("keys"),
		),
		staleCommits: metrix.SeededCounter(meter, "stale_commits",
			metrix.WithDescription("Commits rejected because the key changed during the operation"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("commits"),
		),
		effectBusySeconds: metrix.SeededCounter(meter, "effect_busy_seconds",
			metrix.WithDescription("Cumulative time blocking phases spent executing"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("seconds"),
		),
		suppressedLateOutput: metrix.SeededCounter(meter, "suppressed_late_output",
			metrix.WithDescription("Writes swallowed by closed emission gates after a quarantine"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("writes"),
		),
		barrierWaitSeconds: metrix.SeededCounter(meter, "barrier_wait_seconds",
			metrix.WithDescription("Cumulative time quarantine fences waited on the runtime emission barrier"),
			metrix.WithChartFamily("Agent/JobMgr/Executor"),
			metrix.WithUnit("seconds"),
		),
	}
}

// refreshAges is the runtimecomp producer tick.
func (m *executorRuntimeMetrics) refreshAges() error {
	if m == nil {
		return nil
	}
	m.oldestParkedAge.Set(ageSeconds(m.oldestParkedSince.Load()))
	m.oldestWaitAge.Set(ageSeconds(m.oldestWaitSince.Load()))
	return nil
}

func ageSeconds(sinceNanos int64) float64 {
	if sinceNanos == 0 {
		return 0
	}
	// Clamped: a wall-clock step backwards must not report negative age.
	if s := time.Since(time.Unix(0, sinceNanos)).Seconds(); s > 0 {
		return s
	}
	return 0
}

func (m *Manager) registerExecutorRuntimeComponent() {
	if m.runtimeService == nil || m.executorMetrics == nil {
		return
	}
	err := m.runtimeService.RegisterComponent(runtimecomp.ComponentConfig{
		Name:        executorRuntimeComponentName,
		Store:       m.executorRuntimeStore,
		UpdateEvery: 1,
		Autogen:     runtimecomp.AutogenPolicy{Enabled: true},
		Module:      "jobmgr",
		JobName:     "executor",
		JobLabels:   map[string]string{"component": "jobmgr_executor"},
	})
	if err != nil {
		m.Warningf("executor runtime metrics registration failed: %v", err)
		return
	}
	if err := m.runtimeService.RegisterProducer(executorRuntimeProducerName, m.executorMetrics.refreshAges); err != nil {
		m.Warningf("executor runtime age producer registration failed: %v", err)
	}
}

func (m *Manager) unregisterExecutorRuntimeComponent() {
	if m.runtimeService == nil {
		return
	}
	m.runtimeService.UnregisterProducer(executorRuntimeProducerName)
	m.runtimeService.UnregisterComponent(executorRuntimeComponentName)
}
