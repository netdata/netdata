// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/snmptopologyfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

type funcDepsAdapter struct {
	registry           *topologyRegistry
	diagnosticRecorder *diagnostic.Recorder
}

func (a funcDepsAdapter) Snapshot(options topologyoptions.QueryOptions) (topologyv1.Data, bool, error) {
	if a.registry == nil {
		return topologyv1.Data{}, false, nil
	}

	options = topologyoptions.NormalizeQueryOptions(options)
	generation := a.registry.acquireGeneration()
	capture := beginTopologyGraphDiagnosticCapture(a.diagnosticRecorder, generation, options)
	if capture != nil {
		defer capture.abort()
	}

	dnsCandidates := a.registry.reverseDNSCandidateCollector()
	if dnsCandidates != nil {
		if capture == nil {
			options.ResolveDNSName = dnsCandidates.lookupCached
		} else {
			options.ResolveDNSName = func(ip string) string {
				addr, result, ok := dnsCandidates.lookupCachedResult(ip)
				if !ok {
					return ""
				}
				capture.observeDNS(addr.String(), result)
				if result.State == reversedns.StatePositive {
					return result.Name
				}
				return ""
			}
		}
	}
	if capture != nil {
		lookupVendor := options.LookupVendorByMAC
		if lookupVendor == nil {
			lookupVendor = defaultTopologyVendorLookup
		}
		options.LookupVendorByMAC = func(mac string) (vendor string, prefix string) {
			vendor, prefix = lookupVendor(mac)
			capture.observeOUI(mac, vendor, prefix)
			return vendor, prefix
		}
	}
	data, ok, err := a.registry.snapshotGenerationWithOptions(generation, options)
	if err != nil {
		capture.finish(false, err)
		return topologyv1.Data{}, false, err
	}
	if !ok {
		capture.finish(false, nil)
		return topologyv1.Data{}, false, nil
	}
	if dnsCandidates != nil {
		a.registry.enqueueReverseDNSWarm(dnsCandidates.collectedCandidates())
	}

	payload, err := topologyv1renderer.Render(data)
	if err != nil {
		capture.finish(false, err)
		return topologyv1.Data{}, false, err
	}
	capture.finish(true, nil)
	return payload, true, nil
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
	return snmptopologyfunc.NewHandler(funcDepsAdapter{
		registry: coll.topologyRegistry, diagnosticRecorder: coll.diagnosticRecorder,
	})
}
