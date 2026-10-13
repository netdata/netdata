// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

const (
	writeFailureJournal = telemetry.ErrorJournalWriteFailed
	writeFailureOTLP    = telemetry.ErrorOTLPExportFailed
)

type Job struct {
	policy Policy
	deps   Dependencies

	receiver         *receiver.Receiver
	writer           output.Writer
	journalHost      hostidentity.Provider
	profileLease     *catalog.Lease
	profileIndex     *catalog.Epoch
	telemetry        *telemetry.Job
	deduper          *dedup.Deduper
	dedupKeyVarbinds []string
	packetSequence   atomic.Uint64
	profileMetrics   *profilemetrics.Runtime
	writeFailureDim  telemetry.ErrorKind
	journalActivity  JournalActivityLease

	cleanupOnce sync.Once
}

func New(policy Policy, deps Dependencies) *Job {
	if deps.Catalog == nil {
		panic("snmp_traps jobruntime requires a profile catalog")
	}
	if deps.HostIdentity == nil {
		panic("snmp_traps jobruntime requires host identity")
	}
	if deps.Enricher == nil {
		panic("snmp_traps jobruntime requires enrichment")
	}
	if deps.Telemetry == nil {
		panic("snmp_traps jobruntime requires telemetry")
	}
	if deps.JournalActivity == nil {
		panic("snmp_traps jobruntime requires journal activity")
	}
	return &Job{policy: policy, deps: deps}
}

// Start performs the complete initialization transaction. onCommit runs after
// fallible output startup and before dedup and receiver goroutines start.
func (j *Job) Start(ctx context.Context, onCommit func()) error {
	if j.receiver != nil {
		return nil
	}

	profileLease, err := j.deps.Catalog.Acquire()
	if err != nil {
		return configError(err)
	}
	releaseProfiles := true
	releaseProfileLease := func() {
		if releaseProfiles {
			profileLease.Close()
			releaseProfiles = false
		}
	}

	idx := profileLease.Epoch()
	if idx == nil {
		releaseProfileLease()
		return configError(errors.New("profile index not available"))
	}

	var metricRuntime *profilemetrics.Runtime
	if j.policy.profileMetrics.Enabled() {
		metricRuntime, err = profilemetrics.New(j.policy.profileMetrics, idx, profilemetrics.Options{
			BaseChartTemplateYAML: j.policy.baseChartTemplateYAML,
			SourceHashSalt:        j.sourceHashSalt(),
		})
		if err != nil {
			releaseProfileLease()
			return configError(err)
		}
	}

	var jobTelemetry *telemetry.Job
	reportReceiver := receiver.Reporter(func(event receiver.Event) {
		j.handleReceiverEvent(jobTelemetry, event)
	})
	reportOutput := output.OutcomeReporter(func(outcome output.Outcome) {
		j.handleOutputOutcome(jobTelemetry, outcome)
	})

	prepared, err := j.prepareOutputs(reportOutput)
	if err != nil {
		releaseProfileLease()
		return err
	}

	recv := receiver.New(j.policy.receiver, reportReceiver)
	bindEvents, err := recv.Bind()
	if err != nil {
		releaseProfileLease()
		prepared.close()
		return startupError(err)
	}
	cleanupPreflight := func() {
		releaseProfileLease()
		prepared.close()
		recv.Close()
	}

	if j.policy.receiver.V3Enabled() {
		if j.deps.EngineStateRoot == nil {
			cleanupPreflight()
			return startupError(errors.New("SNMP engine state root is not configured"))
		}
		if err := recv.PrepareV3(j.deps.EngineStateRoot(), j.policy.jobName); err != nil {
			cleanupPreflight()
			if receiver.IsConfigPreparationError(err) {
				return configError(err)
			}
			return startupError(err)
		}
	}

	jobTelemetry = j.attachTelemetry(bindEvents)

	if j.policy.otlpEnabled {
		j.warnPlaintextOTLP()
		if err := prepared.prepareOTLP(ctx, j.policy.jobName, j.policy.otlp, reportOutput); err != nil {
			jobTelemetry.Detach()
			recv.RollbackPreparedState()
			cleanupPreflight()
			return startupError(err)
		}
	}

	writer, writeFailureDim := prepared.coordinator(reportOutput)
	deduper := newDeduper(j.policy.jobName, j.policy.dedup, idx, writer, jobTelemetry, writeFailureDim, func() int64 {
		return j.monotonicUsecWith(prepared.journalHost)
	})

	if err := prepared.start(); err != nil {
		jobTelemetry.Detach()
		recv.RollbackPreparedState()
		cleanupPreflight()
		return startupError(err)
	}

	j.receiver = recv
	j.writer = writer
	j.journalHost = prepared.journalHost
	j.profileLease = profileLease
	j.profileIndex = idx
	j.telemetry = jobTelemetry
	j.deduper = deduper
	j.dedupKeyVarbinds = j.policy.dedup.KeyVarbinds()
	j.profileMetrics = metricRuntime
	j.writeFailureDim = writeFailureDim
	releaseProfiles = false
	if prepared.journal != nil {
		j.journalActivity = j.deps.JournalActivity.Acquire()
	}

	if onCommit != nil {
		onCommit()
	}
	if deduper != nil {
		deduper.Start()
	}
	recv.CommitPreparedState()
	recv.Start(j.handleDatagram)
	return nil
}

func (j *Job) Collect(store metrix.CollectorStore) error {
	if j.receiver == nil || !j.receiver.Ready() {
		return errors.New("receiver not ready")
	}
	j.receiver.Sweep(time.Now())
	if j.telemetry != nil {
		if counter, ok := j.writer.(output.BinaryFieldCounter); ok {
			j.telemetry.SetBinaryEncoded(counter.BinaryEncodedFields())
		}
		j.telemetry.Collect(store)
	}
	if j.profileMetrics != nil {
		j.profileMetrics.Collect(store, j.policy.jobName)
	}
	return nil
}

func (j *Job) ChartTemplateYAML() string {
	if j != nil && j.profileMetrics != nil {
		return j.profileMetrics.ChartTemplateYAML()
	}
	return ""
}

func (j *Job) Cleanup() {
	if j == nil {
		return
	}
	j.cleanupOnce.Do(func() {
		if j.receiver != nil {
			j.receiver.Close()
			j.receiver = nil
		}
		if j.deduper != nil {
			j.deduper.Close()
			j.deduper = nil
		}
		j.dedupKeyVarbinds = nil
		if j.writer != nil {
			_ = j.writer.Close()
			j.writer = nil
		}
		j.journalHost = nil
		if j.profileLease != nil {
			j.profileLease.Close()
			j.profileLease = nil
			j.profileIndex = nil
		}
		if j.journalActivity != nil {
			j.journalActivity.Close()
			j.journalActivity = nil
		}
		if j.telemetry != nil {
			j.telemetry.Detach()
			j.telemetry = nil
		}
		j.profileMetrics = nil
		j.writeFailureDim = ""
	})
}

func (j *Job) sourceHashSalt() string {
	if provider, err := j.deps.HostIdentity.CachedFallback(); err == nil {
		return "machine-id:" + provider.MachineID().String()
	}
	return "netdata-agent"
}

func (j *Job) monotonicUsecWith(provider hostidentity.Provider) int64 {
	if provider != nil {
		return int64(provider.MonotonicUsec())
	}
	provider, err := j.deps.HostIdentity.CachedFallback()
	if err != nil {
		return 0
	}
	return int64(provider.MonotonicUsec())
}
