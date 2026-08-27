// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"reflect"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
)

func (c *Collector) captureTopologyDiagnosticGeneration(generation *topologyGeneration) diagnostic.MemberHandle {
	if c == nil || c.diagnosticRecorder == nil || generation == nil {
		return diagnostic.MemberHandle{}
	}
	dependencies, ok := generation.diagnosticObservationHandles()
	if !ok {
		return diagnostic.MemberHandle{}
	}
	transaction, err := c.diagnosticRecorder.Begin(generation.sequence)
	if err != nil {
		return diagnostic.MemberHandle{}
	}
	abort := true
	defer func() {
		if abort {
			_ = transaction.Abort(diagnostic.GenerationCapabilityV1(), errors.New("generation capture did not commit"))
		}
	}()
	if err := transaction.DefineSection(diagnostic.GenerationSectionGeneration, diagnostic.StateSuccess, 1); err != nil {
		return diagnostic.MemberHandle{}
	}

	base := diagnostic.GenerationV1{
		Sequence:          generation.sequence,
		PublishedAt:       canonicalDiagnosticTime(generation.publishedAt),
		ProducerScopeID:   c.topologyRegistry.producerScope(),
		Kernel:            topologyDiagnosticGraphKernel(),
		DeviceCount:       uint64(len(generation.devices)),
		RenderableDevices: uint64(len(generation.renderableDevices)),
	}
	retainedBytes := diagnosticRetainedBytes(reflect.ValueOf(base)) + uint64(len(dependencies))*512
	var handle diagnostic.MemberHandle
	if len(dependencies) == 0 {
		handle, err = transaction.AddOwned(
			diagnostic.GenerationSectionGeneration,
			diagnostic.MemberType{Kind: diagnostic.KindGeneration, Schema: diagnostic.SchemaV1},
			base,
			retainedBytes,
		)
	} else {
		handle, err = transaction.AddDerivedOwned(
			diagnostic.GenerationSectionGeneration,
			diagnostic.MemberType{Kind: diagnostic.KindGeneration, Schema: diagnostic.SchemaV1},
			dependencies,
			func(refs []diagnostic.ContentRef) (any, error) {
				value := base
				value.Observations = refs
				return value, nil
			},
			retainedBytes,
		)
	}
	if err != nil {
		return diagnostic.MemberHandle{}
	}
	if err := transaction.Commit(diagnostic.GenerationCapabilityV1(), diagnostic.StateSuccess); err != nil {
		return diagnostic.MemberHandle{}
	}
	abort = false
	return handle
}

func (g *topologyGeneration) diagnosticObservationHandles() ([]diagnostic.MemberHandle, bool) {
	if g == nil || len(g.renderableDevices) == 0 {
		return nil, true
	}
	devices := append([]*topologyDeviceGeneration(nil), g.renderableDevices...)
	sort.SliceStable(devices, func(i, j int) bool {
		return compareTopologyObservationSnapshots(devices[i].observation, devices[j].observation) < 0
	})
	handles := make([]diagnostic.MemberHandle, 0, len(devices))
	for _, device := range devices {
		if device == nil || device.diagnosticObservation.ID() == 0 {
			return nil, false
		}
		handles = append(handles, device.diagnosticObservation)
	}
	return handles, true
}

func topologyDiagnosticGraphKernel() diagnostic.GraphKernelV1 {
	return diagnostic.GraphKernelV1{
		Name:         "snmp_topology_graph",
		Revision:     1,
		ModelSchema:  "2.0",
		OutputSchema: "netdata.topology.v1",
	}
}
