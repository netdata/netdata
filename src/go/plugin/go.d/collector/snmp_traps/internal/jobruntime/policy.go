// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

type Override struct {
	OID      string
	Category string
	Severity string
	Labels   map[string]string
}

type PolicyConfig struct {
	JobName               string
	Receiver              receiver.Policy
	JournalEnabled        bool
	Journal               journal.Config
	OTLPEnabled           bool
	OTLP                  otlp.Policy
	Dedup                 dedup.Policy
	ProfileMetrics        profilemetrics.Policy
	ReverseDNSEnabled     bool
	Overrides             []Override
	BaseChartTemplateYAML string
}

// Policy is an immutable, normalized snapshot of one job's runtime settings.
type Policy struct {
	jobName               string
	receiver              receiver.Policy
	journalEnabled        bool
	journal               journal.Config
	otlpEnabled           bool
	otlp                  otlp.Policy
	dedup                 dedup.Policy
	profileMetrics        profilemetrics.Policy
	reverseDNSEnabled     bool
	overrides             map[string]Override
	baseChartTemplateYAML string
}

func NewPolicy(cfg PolicyConfig) Policy {
	overrides := make(map[string]Override, len(cfg.Overrides))
	for _, src := range cfg.Overrides {
		ov := src
		ov.Labels = maps.Clone(src.Labels)
		overrides[ov.OID] = ov
	}
	if len(overrides) == 0 {
		overrides = nil
	}
	return Policy{
		jobName:               cfg.JobName,
		receiver:              cfg.Receiver,
		journalEnabled:        cfg.JournalEnabled,
		journal:               cfg.Journal,
		otlpEnabled:           cfg.OTLPEnabled,
		otlp:                  cfg.OTLP,
		dedup:                 cfg.Dedup,
		profileMetrics:        cfg.ProfileMetrics,
		reverseDNSEnabled:     cfg.ReverseDNSEnabled,
		overrides:             overrides,
		baseChartTemplateYAML: cfg.BaseChartTemplateYAML,
	}
}
