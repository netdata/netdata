// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import "github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"

// eventPlan is the executor-local scheduling plan for one event. Domain
// controllers describe command shape; jobmgr attaches claim computation because
// claim keys and modes are executor-owned.
type eventPlan struct {
	command dyncfg.CommandPlan
	claims  func() []claim
}

func newClaimlessEventPlan() eventPlan {
	return eventPlan{command: dyncfg.CommandPlanClaimless()}
}

func newClaimedEventPlan(claims func() []claim) eventPlan {
	return eventPlan{command: dyncfg.CommandPlanClaims(), claims: claims}
}

func newDyncfgEventPlan(command dyncfg.CommandPlan, claims func() []claim) eventPlan {
	return eventPlan{command: command, claims: claims}
}

func (p eventPlan) needsClaims() bool {
	return p.command.NeedsClaims(false)
}

func (p eventPlan) bypassesForeignWriteHold() bool {
	return !p.command.NeedsClaims(true)
}

func (p eventPlan) computeClaims() []claim {
	if !p.needsClaims() || p.claims == nil {
		return nil
	}
	return p.claims()
}

func (e *executor) planEvent(ev event) eventPlan {
	switch ev.kind {
	case eventDiscoveryAdd, eventDiscoveryRemove:
		return newClaimedEventPlan(func() []claim { return e.eventClaims(ev) })
	case eventDyncfgCommand:
		return e.planDyncfgCommand(ev)
	default:
		return newClaimlessEventPlan()
	}
}

func (e *executor) planDyncfgCommand(ev event) eventPlan {
	if ev.underivable {
		return newClaimlessEventPlan()
	}

	var plan dyncfg.CommandPlan
	switch ev.domain {
	case domainSecretStore:
		plan = e.mgr.secretsCtl.CommandPlan(ev.fn)
	case domainVnode:
		plan = e.mgr.vnodesCtl.CommandPlan(ev.fn)
	case domainCollector:
		entry, ok := e.mgr.collectorExposed.LookupByKey(ev.key)
		if !ok {
			entry = nil
		}
		plan = e.mgr.collectorHandler.CommandPlan(ev.fn, entry)
	default:
		return newClaimlessEventPlan()
	}

	return newDyncfgEventPlan(plan, func() []claim { return e.eventClaims(ev) })
}
