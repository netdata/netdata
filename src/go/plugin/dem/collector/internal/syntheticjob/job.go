// SPDX-License-Identifier: GPL-3.0-or-later

// Package syntheticjob owns the lifecycle shared by the journey and lighthouse collectors: preparation, rehearsal,
// active registration, the monitor session, vnode association, and the attempt and kind metrics. A collector embeds
// Job and keeps its configuration and request construction.
package syntheticjob

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// Executor is the process browser capability injected into native synthetic collectors. It owns process-wide
// admission and supervised completion. Drained=false requires process fail-stop even if the collector has already
// retired.
type Executor interface {
	Check(context.Context) error
	monitor.Executor
	Rehearse(context.Context, synthetic.Request) error
}

type Dependencies struct {
	Executor Executor
	Registry *registry.Registry
	// ActivationDependencies names the process browser runtime that jobs wait
	// for before activation; nil when the runtime mode has no readiness owner.
	ActivationDependencies func(map[string]any) ([]collectorapi.ActivationDependency, error)
}

var errNotInitialized = errors.New("collector is not initialized")

// Job is embedded by a synthetic collector. Configured vnode snapshots, Init, Check and Collect share the framework
// job goroutine.
type Job struct {
	deps        Dependencies
	store       metrix.CollectorStore
	attempt     attemptMetrics
	kind        kindMetrics
	request     synthetic.Request
	checkEvery  int
	initialized bool
	vnodeGUID   string
	// Assigned before readiness; Run owns cancellation and physical completion.
	session *monitor.Session
}

// New creates the job metric store with the attempt metrics and the measurements of kind.
func New(deps Dependencies, kind synthetic.Kind) Job {
	store := metrix.NewCollectorStore()
	meter := store.Write().SnapshotMeter("")
	return Job{
		deps:    deps,
		store:   store,
		attempt: newAttemptMetrics(meter),
		kind:    newKindMetrics(kind, meter),
	}
}

func (j *Job) MetricStore() metrix.CollectorStore { return j.store }

// Cleanup has nothing to release: Run owns the registration and the monitor session.
func (j *Job) Cleanup(context.Context) {}

// Rehearse runs the initialized request once without registering it or publishing metrics.
func (j *Job) Rehearse(ctx context.Context) error {
	if !j.initialized {
		return errNotInitialized
	}
	return j.deps.Executor.Rehearse(ctx, j.request)
}
