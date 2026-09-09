// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

type preparedOutputs struct {
	journal     *journal.Writer
	otlp        *otlp.Writer
	journalHost hostidentity.Provider
}

func (j *Job) prepareOutputs(report output.OutcomeReporter) (*preparedOutputs, error) {
	prepared := &preparedOutputs{}
	if !j.policy.journalEnabled {
		return prepared, nil
	}
	if err := journal.ValidateLogRoot(); err != nil {
		return nil, startupError(err)
	}
	host, err := j.deps.HostIdentity.FreshJournal()
	if err != nil {
		return nil, startupError(err)
	}
	prepared.journalHost = host
	prepared.journal, err = journal.Prepare(
		journal.Root(j.policy.jobName),
		j.policy.journal,
		host,
		journal.Options{Report: report},
	)
	if err != nil {
		return nil, startupError(err)
	}
	return prepared, nil
}

func (p *preparedOutputs) prepareOTLP(ctx context.Context, jobName string, policy otlp.Policy, report output.OutcomeReporter) error {
	writer, err := otlp.Prepare(ctx, jobName, policy, otlp.Options{
		Authoritative: p.journal == nil,
		Report:        report,
	})
	if err != nil {
		return err
	}
	p.otlp = writer
	return nil
}

func (p *preparedOutputs) coordinator(report output.OutcomeReporter) (output.Writer, telemetry.ErrorKind) {
	var primary output.Writer
	if p.journal != nil {
		primary = p.journal
	}
	var secondary output.Writer
	if p.otlp != nil {
		secondary = p.otlp
	}
	writeFailure := writeFailureJournal
	if primary == nil {
		writeFailure = writeFailureOTLP
	}
	return output.NewCoordinator(primary, secondary, output.BackendOTLP, report), writeFailure
}

func (p *preparedOutputs) start() error {
	var otlpStarter, journalStarter outputStarter
	if p.otlp != nil {
		otlpStarter = p.otlp
	}
	if p.journal != nil {
		journalStarter = p.journal
	}
	return startOutputBackends(otlpStarter, journalStarter)
}

type outputStarter interface {
	Start() error
}

func startOutputBackends(otlpBackend, journalBackend outputStarter) error {
	if otlpBackend != nil {
		if err := otlpBackend.Start(); err != nil {
			return err
		}
	}
	if journalBackend != nil {
		if err := journalBackend.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (p *preparedOutputs) close() {
	if p == nil {
		return
	}
	if p.journal != nil {
		_ = p.journal.Close()
	}
	if p.otlp != nil {
		_ = p.otlp.Close()
	}
}
