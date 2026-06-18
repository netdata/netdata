// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
)

type funcDepsAdapter struct {
	registry *topologyRegistry
}

func (a funcDepsAdapter) Snapshot(options snmptopologyfunc.QueryOptions) (topologyv1.Data, bool, error) {
	if a.registry == nil {
		return topologyv1.Data{}, false, nil
	}

	data, ok := a.registry.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                options.MapType,
		InferenceStrategy:      options.InferenceStrategy,
		ManagedDeviceFocus:     options.ManagedDeviceFocus,
		Depth:                  options.Depth,
		ResolveDNSName:         resolveTopologyReverseDNSNameNoop, // never block on network I/O
	})
	if !ok {
		return topologyv1.Data{}, false, nil
	}

	payload, err := snmpTopologyToV1(data)
	if err != nil {
		return topologyv1.Data{}, false, err
	}
	return payload, true, nil
}

func (a funcDepsAdapter) ManagedDeviceFocusTargets() []snmptopologyfunc.ManagedFocusTarget {
	if a.registry == nil {
		return nil
	}

	targets := a.registry.managedDeviceFocusTargets()
	out := make([]snmptopologyfunc.ManagedFocusTarget, 0, len(targets))
	for _, target := range targets {
		out = append(out, snmptopologyfunc.ManagedFocusTarget{
			Value: target.Value,
			Name:  target.Name,
		})
	}
	return out
}

func topologyMethods() []funcapi.MethodConfig {
	return snmptopologyfunc.Methods()
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
