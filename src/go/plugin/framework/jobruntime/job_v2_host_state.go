// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"maps"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type jobV2HostKind uint8

const (
	jobV2HostUnset jobV2HostKind = iota
	jobV2HostGlobal
	jobV2HostVnode
)

type jobV2HostRef struct {
	kind jobV2HostKind
	guid string
}

func jobV2HostFromVnode(vnode vnodes.VirtualNode) jobV2HostRef {
	if vnode.GUID == "" {
		return jobV2HostRef{kind: jobV2HostGlobal}
	}
	return jobV2HostRef{kind: jobV2HostVnode, guid: vnode.GUID}
}

func (r jobV2HostRef) isSet() bool    { return r.kind != jobV2HostUnset }
func (r jobV2HostRef) isGlobal() bool { return r.kind == jobV2HostGlobal }
func (r jobV2HostRef) isVnode() bool  { return r.kind == jobV2HostVnode }

type jobV2EmissionDecision struct {
	targetHost       jobV2HostRef
	needEngineReload bool
	hostScope        *chartemit.HostScope
	defineEmitted    bool
	defineInfo       netdataapi.HostInfo
}

type jobV2HostState struct {
	definedHost   jobV2HostRef
	definedInfo   netdataapi.HostInfo
	engineHost    jobV2HostRef
	cleanupOwner  jobV2HostRef
	cleanupCharts map[string]chartengine.ChartMeta
}

func (s *jobV2HostState) invalidateDefine() {
	if s == nil {
		return
	}
	s.definedHost = jobV2HostRef{}
	s.definedInfo = netdataapi.HostInfo{}
}

func (s *jobV2HostState) prepareEmission(vnode vnodes.VirtualNode) (jobV2EmissionDecision, error) {
	target := jobV2HostFromVnode(vnode)
	decision := jobV2EmissionDecision{
		targetHost:       target,
		needEngineReload: s != nil && s.engineHost.isSet() && s.engineHost != target,
	}
	if target.isGlobal() {
		return decision, nil
	}

	info, needDefine, err := s.prepareDefine(vnode, target)
	if err != nil {
		return jobV2EmissionDecision{}, err
	}
	scope := &chartemit.HostScope{GUID: target.guid}
	if needDefine {
		scope.Define = &info
	}
	decision.hostScope = scope
	decision.defineEmitted = needDefine
	decision.defineInfo = info
	return decision, nil
}

func (s *jobV2HostState) prepareDefine(vnode vnodes.VirtualNode, target jobV2HostRef) (netdataapi.HostInfo, bool, error) {
	info, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     vnode.GUID,
		Hostname: vnode.Hostname,
		Labels:   vnode.Labels,
	})
	if err != nil {
		return netdataapi.HostInfo{}, false, err
	}
	if s != nil && s.definedHost == target && hostInfoEqual(s.definedInfo, info) {
		return netdataapi.HostInfo{}, false, nil
	}
	return info, true, nil
}

func (s *jobV2HostState) onEngineReload(target jobV2HostRef) {
	if s == nil {
		return
	}
	s.engineHost = target
}

func (s *jobV2HostState) commitSuccessfulEmission(plan chartengine.Plan, decision jobV2EmissionDecision) {
	if s == nil || len(plan.Actions) == 0 {
		return
	}
	s.engineHost = decision.targetHost
	if decision.defineEmitted {
		s.definedHost = decision.targetHost
		s.definedInfo = decision.defineInfo
	}
	if s.cleanupCharts == nil {
		s.cleanupCharts = make(map[string]chartengine.ChartMeta)
	}

	createCharts := make(map[string]chartengine.ChartMeta)
	dimensionOnlyCharts := make(map[string]chartengine.ChartMeta)
	removeCharts := make(map[string]struct{})

	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			createCharts[v.ChartID] = v.Meta
		case chartengine.CreateDimensionAction:
			if _, ok := createCharts[v.ChartID]; ok {
				continue
			}
			if _, ok := dimensionOnlyCharts[v.ChartID]; !ok {
				dimensionOnlyCharts[v.ChartID] = v.ChartMeta
			}
		case chartengine.RemoveChartAction:
			removeCharts[v.ChartID] = struct{}{}
		}
	}

	maps.Copy(s.cleanupCharts, createCharts)
	for chartID, meta := range dimensionOnlyCharts {
		if _, ok := s.cleanupCharts[chartID]; ok {
			continue
		}
		s.cleanupCharts[chartID] = meta
	}
	for chartID := range removeCharts {
		delete(s.cleanupCharts, chartID)
	}

	s.cleanupOwner = decision.targetHost
}

func hostInfoEqual(left, right netdataapi.HostInfo) bool {
	return left.GUID == right.GUID &&
		left.Hostname == right.Hostname &&
		maps.Equal(left.Labels, right.Labels)
}

func (r jobV2HostRef) String() string {
	switch r.kind {
	case jobV2HostUnset:
		return "unset"
	case jobV2HostGlobal:
		return "global"
	case jobV2HostVnode:
		return fmt.Sprintf("vnode(%s)", r.guid)
	default:
		return "unknown"
	}
}
