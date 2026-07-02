// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// The executor is the single dispatch seam between the run loop and the
// command handlers: every discovery and dyncfg event is classified into an
// event (kind, domain, derived key) and executed through dispatch, so
// execution policy can change without touching the handlers or the loop's
// select structure.
//
// The current policy is one global lane: dispatch runs every event inline on
// the caller goroutine, with no queuing, preserving the run loop's serialized
// semantics. The derived key identifies the config an event addresses and is
// inert routing metadata under this policy.

type eventKind int8

const (
	eventDiscoveryAdd eventKind = iota + 1
	eventDiscoveryRemove
	eventDyncfgCommand
)

type eventDomain int8

const (
	domainUnknown eventDomain = iota
	domainCollector
	domainSecretStore
	domainVnode
)

// event is one unit of work for the executor. Discovery events carry cfg;
// dyncfg command events carry fn.
type event struct {
	kind   eventKind
	domain eventDomain
	key    string
	cfg    confgroup.Config
	fn     dyncfg.Function
}

type executor struct {
	mgr *Manager
}

func newExecutor(mgr *Manager) *executor {
	return &executor{mgr: mgr}
}

// dispatch executes ev synchronously on the caller goroutine.
func (e *executor) dispatch(ev event) {
	switch ev.kind {
	case eventDiscoveryAdd:
		e.mgr.addConfig(ev.cfg)
	case eventDiscoveryRemove:
		e.mgr.removeConfig(ev.cfg)
	case eventDyncfgCommand:
		e.dispatchDyncfg(ev)
	}
}

func (e *executor) dispatchDyncfg(ev event) {
	switch ev.domain {
	case domainSecretStore:
		e.mgr.dyncfgSecretStoreSeqExec(ev.fn)
	case domainCollector:
		e.mgr.dyncfgCollectorSeqExec(ev.fn)
	case domainVnode:
		e.mgr.dyncfgVnodeSeqExec(ev.fn)
	default:
		e.mgr.dyncfgRespondUnknown(ev.fn)
	}
}

func (m *Manager) newDiscoveryAddEvent(cfg confgroup.Config) event {
	return event{kind: eventDiscoveryAdd, domain: domainCollector, key: cfg.ExposedKey(), cfg: cfg}
}

func (m *Manager) newDiscoveryRemoveEvent(cfg confgroup.Config) event {
	return event{kind: eventDiscoveryRemove, domain: domainCollector, key: cfg.ExposedKey(), cfg: cfg}
}

// newDyncfgEvent classifies a dyncfg command by ID prefix and derives its
// key. Underivable input keeps the domain's fallback key and still executes,
// so it reaches the handler's existing rejection paths unchanged.
func (m *Manager) newDyncfgEvent(fn dyncfg.Function) event {
	ev := event{kind: eventDyncfgCommand, domain: m.dyncfgDomain(fn), fn: fn}

	var key string
	var ok bool
	switch ev.domain {
	case domainSecretStore:
		key, ok = m.secretsCtl.DeriveKey(fn)
	case domainCollector:
		key, _, _, ok = m.collectorCommandKey(fn)
	case domainVnode:
		key, ok = m.vnodesCtl.DeriveKey(fn)
	}
	if !ok {
		key = m.dyncfgFallbackKey(ev.domain)
	}
	ev.key = key
	return ev
}

func (m *Manager) dyncfgDomain(fn dyncfg.Function) eventDomain {
	switch {
	case strings.HasPrefix(fn.ID(), m.dyncfgSecretStorePrefixValue()):
		return domainSecretStore
	case strings.HasPrefix(fn.ID(), m.dyncfgCollectorPrefixValue()):
		return domainCollector
	case strings.HasPrefix(fn.ID(), m.dyncfgVnodePrefixValue()):
		return domainVnode
	default:
		return domainUnknown
	}
}

// dyncfgFallbackKey is the registration-wide key for commands whose config
// identity cannot be derived; unknown-prefix commands are rejected at
// dispatch and get no domain key.
func (m *Manager) dyncfgFallbackKey(domain eventDomain) string {
	switch domain {
	case domainSecretStore:
		return m.dyncfgSecretStorePrefixValue()
	case domainCollector:
		return m.dyncfgCollectorPrefixValue()
	case domainVnode:
		return m.dyncfgVnodePrefixValue()
	default:
		return ""
	}
}
