// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
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
	targetHost           jobV2HostRef
	needEngineReload     bool
	hostScope            *chartemit.HostScope
	defineInfo           netdataapi.HostInfo
	registryOwner        vnoderegistry.Owner
	registryRegistration vnoderegistry.Registration
}

type jobV2HostState struct {
	definedHost   jobV2HostRef
	definedInfo   netdataapi.HostInfo
	engineHost    jobV2HostRef
	cleanupOwner  jobV2HostRef
	cleanupCharts map[string]chartengine.ChartMeta
	// registryOwners tracks successfully emitted vnode owners so cleanup can
	// release them after obsolete-chart emission.
	registryOwners map[vnoderegistry.Owner]string
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

	scope := &chartemit.HostScope{GUID: target.guid}
	decision.hostScope = scope
	return decision, nil
}

func (s *jobV2HostState) prepareScopedEmission(scope metrix.HostScope) (jobV2EmissionDecision, error) {
	target := jobV2HostRef{kind: jobV2HostVnode, guid: scope.GUID}
	decision := jobV2EmissionDecision{
		targetHost:       target,
		needEngineReload: s != nil && s.engineHost.isSet() && s.engineHost != target,
		hostScope:        &chartemit.HostScope{GUID: target.guid},
	}
	return decision, nil
}

func (s *jobV2HostState) commitSuccessfulEmission(plan chartengine.Plan, decision jobV2EmissionDecision) {
	if s == nil {
		return
	}
	if len(plan.Actions) == 0 {
		if decision.needEngineReload {
			// Quiet host switches still need to finish after the scope attempt commits.
			s.engineHost = decision.targetHost
		}
		return
	}
	s.engineHost = decision.targetHost
	if decision.hostScope != nil {
		s.definedHost = decision.targetHost
		s.definedInfo = decision.defineInfo
	}
	if decision.registryOwner != "" {
		if s.registryOwners == nil {
			s.registryOwners = make(map[vnoderegistry.Owner]string)
		}
		s.registryOwners[decision.registryOwner] = decision.targetHost.guid
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

func (s *jobV2HostState) releaseRegistryOwners(registry *vnoderegistry.Registry) {
	if s == nil || registry == nil || len(s.registryOwners) == 0 {
		return
	}
	for owner, guid := range s.registryOwners {
		registry.Release(owner, guid)
		delete(s.registryOwners, owner)
	}
}

func (s *jobV2HostState) releaseSupersededRegistryOwnersExcept(registry *vnoderegistry.Registry, current map[vnoderegistry.Owner]struct{}, ownerPrefix string) {
	if s == nil || registry == nil || len(s.registryOwners) == 0 {
		return
	}
	for owner, guid := range s.registryOwners {
		if _, ok := current[owner]; ok || !strings.HasPrefix(string(owner), ownerPrefix) {
			continue
		}
		registry.Release(owner, guid)
		delete(s.registryOwners, owner)
	}
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
