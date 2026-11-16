// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
)

type funcDepsAdapter struct {
	registry *topologyRegistry
}

func (a funcDepsAdapter) Snapshot(options topologyoptions.QueryOptions) (snmptopologyfunc.SnapshotResult, bool, error) {
	if a.registry == nil {
		return snmptopologyfunc.SnapshotResult{}, false, nil
	}

	dnsCandidates := a.registry.reverseDNSCandidateCollector()
	environment := topologyGraphBuildEnvironment{}
	if dnsCandidates != nil {
		environment.resolveDNSName = dnsCandidates.lookupCached
	}
	result, ok, err := a.registry.snapshotResultWithEnvironment(options, environment)
	if err != nil {
		return snmptopologyfunc.SnapshotResult{}, false, err
	}
	if !ok {
		return snmptopologyfunc.SnapshotResult{}, false, nil
	}
	if dnsCandidates != nil {
		a.registry.enqueueReverseDNSWarm(dnsCandidates.collectedCandidates())
	}

	payload, err := topologyv1renderer.Render(result.Data)
	if err != nil {
		return snmptopologyfunc.SnapshotResult{}, false, err
	}
	return snmptopologyfunc.SnapshotResult{Data: payload, FocusTargets: result.FocusTargets}, true, nil
}

func (a funcDepsAdapter) ManagedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget {
	if a.registry == nil {
		return nil
	}
	return a.registry.managedDeviceFocusTargets()
}

func topologyMethods() []funcapi.FunctionConfig {
	return snmptopologyfunc.Methods()
}

func (c *Collector) FunctionAvailable(functionID string) bool {
	if c == nil || functionID != snmptopologyfunc.MethodID || c.topologyRegistry == nil {
		return false
	}
	return c.topologyRegistry.hasRenderableObservations()
}

func topologyFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	if job == nil {
		return nil
	}
	coll, ok := job.Collector().(*Collector)
	if !ok || coll == nil {
		return nil
	}
	return snmptopologyfunc.NewHandler(funcDepsAdapter{registry: coll.topologyRegistry})
}
