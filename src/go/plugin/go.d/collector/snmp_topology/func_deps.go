// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
)

type funcDepsAdapter struct {
	registry *topologyRegistry
}

func (a funcDepsAdapter) Snapshot(options topologyoptions.QueryOptions) (topologyv1.Data, bool, error) {
	if a.registry == nil {
		return topologyv1.Data{}, false, nil
	}

	dnsCandidates := a.registry.reverseDNSCandidateCollector()
	if dnsCandidates != nil {
		options.ResolveDNSName = dnsCandidates.lookupCached
	}
	data, ok := a.registry.snapshotWithOptions(options)
	if !ok {
		return topologyv1.Data{}, false, nil
	}
	if dnsCandidates != nil {
		a.registry.enqueueReverseDNSWarm(dnsCandidates.collectedCandidates())
	}

	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	return payload, true, nil
}

func (a funcDepsAdapter) ManagedDeviceFocusTargets() []topologyoptions.ManagedFocusTarget {
	if a.registry == nil {
		return nil
	}
	return a.registry.managedDeviceFocusTargets()
}

func topologyMethods(availability *topologyFunctionAvailability) []funcapi.MethodConfig {
	methods := snmptopologyfunc.Methods()
	for i := range methods {
		if methods[i].ID == snmptopologyfunc.MethodID {
			methods[i].Available = availability.Available
		}
	}
	return methods
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
