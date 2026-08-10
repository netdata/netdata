// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

const (
	listenerReadErrorLogEvery     = time.Hour
	listenerReadErrorLogKeyPrefix = "snmp_traps:listener_read_failed:"
)

func (j *Job) attachTelemetry(bindEvents []receiver.Event) *telemetry.Job {
	jobTelemetry := j.deps.Telemetry.Attach(j.policy.jobName, telemetry.Options{DedupEnabled: j.policy.dedup.Enabled()})
	for _, event := range bindEvents {
		j.handleReceiverEvent(jobTelemetry, event)
	}
	return jobTelemetry
}

func (j *Job) handleOutputOutcome(jobTelemetry *telemetry.Job, outcome output.Outcome) {
	if outcome.Backend == output.BackendJournal && outcome.Err != nil {
		j.deps.Log.warningf("SNMP trap journal writer stopped for job %q: %v", j.policy.jobName, outcome.Err)
	}
	if jobTelemetry == nil || outcome.FailedEntries == 0 {
		return
	}
	switch outcome.Backend {
	case output.BackendJournal:
		jobTelemetry.AddError(writeFailureJournal, outcome.FailedEntries)
	case output.BackendOTLP:
		jobTelemetry.AddError(writeFailureOTLP, outcome.FailedEntries)
	}
	if outcome.Authoritative {
		jobTelemetry.PipelineWriteFailed(outcome.FailedEntries)
	}
}

func (j *Job) warnPlaintextOTLP() {
	if j.policy.otlp.PlaintextRemote() {
		j.deps.Log.warningf("SNMP trap OTLP endpoint %q uses plaintext transport; use https:// for remote collectors", j.policy.otlp.Target())
	}
}

func (j *Job) handleReceiverEvent(jobTelemetry *telemetry.Job, event receiver.Event) {
	switch event.Type {
	case receiver.EventError:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorKind(event.ErrorKind))
		}
	case receiver.EventDecoded:
		if jobTelemetry != nil {
			jobTelemetry.PipelineDecoded()
		}
	case receiver.EventListenerReadFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorListenerReadFailed)
		}
		endpoint := event.Endpoint.LogName()
		j.deps.Log.warnLimited(
			listenerReadErrorLogKeyPrefix+endpoint,
			listenerReadErrorLogEvery,
			"SNMP trap listener read failed (endpoint=%s): %v",
			endpoint,
			event.Err,
		)
	case receiver.EventListenerBufferDegraded:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorListenerBufferDegraded)
		}
		j.deps.Log.warningf(
			"SNMP trap listener receive buffer request degraded (endpoint=%s, requested=%d bytes): %v; continuing with the OS socket buffer, high trap bursts may be dropped",
			event.Endpoint.LogName(), event.Requested, event.Err,
		)
	case receiver.EventDynamicEngineIDRegistered:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorUnknownEngineID)
		}
		j.deps.Log.warningf(
			"Dynamic SNMPv3 engine ID registered: engineID=%s username=%s. This sender was not in the static configuration. Every first-time dynamic (engineID, username) pair is accepted and logged once per job lifetime.",
			event.EngineID, event.Username,
		)
	case receiver.EventInformResponseFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorInformResponseFailed)
		}
		j.deps.Log.warningf("SNMP trap INFORM response failed: %v", event.Err)
	case receiver.EventDiscoveryReportFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorInformResponseFailed)
		}
		j.deps.Log.warningf("SNMP trap INFORM discovery Report failed: %v", event.Err)
	}
}
