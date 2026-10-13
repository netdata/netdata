// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

type JournalActivity interface {
	Acquire() JournalActivityLease
}

type HostIdentity interface {
	FreshJournal() (hostidentity.Provider, error)
	CachedFallback() (hostidentity.Provider, error)
}

type JournalActivityLease interface {
	Close()
}

type Logger struct {
	Warningf    func(format string, args ...any)
	Errorf      func(format string, args ...any)
	WarnLimited func(key string, every time.Duration, format string, args ...any)
}

func (l Logger) warningf(format string, args ...any) {
	if l.Warningf != nil {
		l.Warningf(format, args...)
	}
}

func (l Logger) errorf(format string, args ...any) {
	if l.Errorf != nil {
		l.Errorf(format, args...)
	}
}

func (l Logger) warnLimited(key string, every time.Duration, format string, args ...any) {
	if l.WarnLimited != nil {
		l.WarnLimited(key, every, format, args...)
	}
}

type Dependencies struct {
	Catalog         *catalog.Manager
	HostIdentity    HostIdentity
	Enricher        *enrichment.Enricher
	Telemetry       *telemetry.Registry
	JournalActivity JournalActivity
	EngineStateRoot func() string
	Log             Logger
}
