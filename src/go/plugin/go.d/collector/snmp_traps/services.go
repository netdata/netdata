// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

type pluginServices struct {
	enricher          *enrichment.Enricher
	hostIdentity      jobruntime.HostIdentity
	telemetryRegistry *telemetry.Registry
	journalActivity   *journalActivity
	engineStateRoot   func() string

	catalogMu sync.Mutex
	catalog   *catalog.Manager
}

func newPluginServices(enricher *enrichment.Enricher, hostIdentity jobruntime.HostIdentity, telemetryRegistry *telemetry.Registry, engineStateRoot func() string) *pluginServices {
	return &pluginServices{
		enricher:          enricher,
		hostIdentity:      hostIdentity,
		telemetryRegistry: telemetryRegistry,
		journalActivity:   &journalActivity{},
		engineStateRoot:   engineStateRoot,
	}
}

func (s *pluginServices) primeCatalog() {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.catalog == nil {
		s.catalog = catalog.NewManager(defaultProfileCatalogPaths())
	}
}

// catalogCandidate preserves standalone New semantics: a failed Init does not
// freeze paths, while registered creators prime and share one manager.
func (s *pluginServices) catalogCandidate() (*catalog.Manager, func()) {
	s.catalogMu.Lock()
	if s.catalog != nil {
		manager := s.catalog
		s.catalogMu.Unlock()
		return manager, func() {}
	}
	s.catalogMu.Unlock()

	manager := catalog.NewManager(defaultProfileCatalogPaths())
	return manager, func() {
		s.catalogMu.Lock()
		if s.catalog == nil {
			s.catalog = manager
		}
		s.catalogMu.Unlock()
	}
}

func (s *pluginServices) dependencies(c *Collector, manager *catalog.Manager) jobruntime.Dependencies {
	return jobruntime.Dependencies{
		Catalog:         manager,
		HostIdentity:    s.hostIdentity,
		Enricher:        s.enricher,
		Telemetry:       s.telemetryRegistry,
		JournalActivity: s.journalActivity,
		EngineStateRoot: s.engineStateRoot,
		Log: jobruntime.Logger{
			Warningf: func(format string, args ...any) {
				if c.Logger != nil {
					c.Warningf(format, args...)
				}
			},
			Errorf: func(format string, args ...any) {
				if c.Logger != nil {
					c.Errorf(format, args...)
				}
			},
			WarnLimited: func(key string, every time.Duration, format string, args ...any) {
				if c.Logger != nil {
					c.Limit(key, 1, every).Warningf(format, args...)
				}
			},
		},
	}
}

type journalActivity struct {
	active atomic.Int64
}

func (a *journalActivity) Acquire() jobruntime.JournalActivityLease {
	a.active.Add(1)
	return &journalActivityLease{owner: a}
}

func (a *journalActivity) Available() bool {
	return a != nil && a.active.Load() > 0
}

type journalActivityLease struct {
	once  sync.Once
	owner *journalActivity
}

func (l *journalActivityLease) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.owner != nil {
			l.owner.active.Add(-1)
			l.owner = nil
		}
	})
}
