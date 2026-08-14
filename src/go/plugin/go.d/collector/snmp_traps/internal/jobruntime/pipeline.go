// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

func (j *Job) handleDatagram(datagram receiver.Datagram) {
	peerIP := datagram.PeerIP
	packetSequence := j.packetSequence.Add(1)
	jobTelemetry := j.telemetry
	jobTelemetry.PipelineReceived()
	packetFinished := false

	defer func() {
		if v := recover(); v != nil {
			jobTelemetry.Error(telemetry.ErrorDecodeFailed)
			if !packetFinished {
				jobTelemetry.PipelineDropped()
			}
			j.deps.Log.errorf("SNMP trap packet handling panic from %s: %v", peerIP, v)
			return
		}
		if !packetFinished {
			jobTelemetry.PipelineDropped()
		}
	}()

	result := j.receiver.Process(datagram)
	if failure := result.DecodeFailure; failure != nil {
		if j.writer != nil && j.receiver.AdmitDecodeErrorAudit(failure) {
			j.writeDecodeErrorEntry(failure, packetSequence)
		}
		return
	}
	if result.PDU == nil {
		return
	}

	pdu := result.PDU
	var td *catalog.TrapDef
	var profileLookupErr error
	if j.profileIndex != nil {
		td, profileLookupErr = j.profileIndex.LookupWithError(pdu.OID)
		if profileLookupErr != nil {
			j.deps.Log.warningf("SNMP trap profile lookup failed for OID %s: %v", pdu.OID, profileLookupErr)
			jobTelemetry.Error(telemetry.ErrorProfileLoadFailed)
		}
	}
	unknownOID := td == nil && profileLookupErr == nil
	if td != nil {
		td = j.applyOverrides(td)
	}

	entry := entryFromPDU(j.policy.jobName, pdu, td, time.Now().UnixMicro(), j.monotonicUsecWith(j.journalHost))
	entry.PacketSequence = packetSequence
	j.deps.Enricher.Enrich(entry, j.policy.reverseDNSEnabled)
	renderEntryTemplates(entry, td)
	if unknownOID {
		jobTelemetry.Error(telemetry.ErrorUnknownOID)
	}
	if catalog.EntryHasUnresolvedTemplate(entry) {
		jobTelemetry.Error(telemetry.ErrorTemplateUnresolved)
	}
	jobTelemetry.PipelineAccepted()

	var admission dedup.Admission
	if j.deduper != nil {
		var decision dedup.Decision
		admission, decision = j.deduper.Admit(entry, selectDedupKeyVarbinds(td, j.dedupKeyVarbinds))
		if decision == dedup.DecisionSuppress {
			jobTelemetry.DedupSuppressed()
			packetFinished = true
			return
		}
	}
	if err := j.writer.Write(entry); err != nil {
		if j.deduper != nil {
			j.deduper.Rollback(admission)
		}
		jobTelemetry.Error(j.trapWriteFailureDim())
		jobTelemetry.PipelineWriteFailed(1)
		packetFinished = true
		return
	}
	packetFinished = true

	if j.profileMetrics != nil {
		j.profileMetrics.Update(entry)
	}

	category := model.Category("unknown")
	if td != nil {
		category = model.Category(td.Category)
	}
	jobTelemetry.PipelineCommitted()
	jobTelemetry.Event(category)
	jobTelemetry.Severity(entry.Severity)
}

func (j *Job) applyOverrides(td *catalog.TrapDef) *catalog.TrapDef {
	if td == nil || len(j.policy.overrides) == 0 {
		return td
	}
	ov, ok := j.policy.overrides[td.OID]
	if !ok {
		return td
	}
	return td.WithOverrides(ov.Category, ov.Severity, ov.Labels)
}

func entryFromPDU(jobName string, pdu *model.TrapPDU, td *catalog.TrapDef, realtimeUsec, monotonicUsec int64) *model.TrapEntry {
	entry := &model.TrapEntry{
		JobName:               jobName,
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  realtimeUsec,
		ReceivedMonotonicUsec: monotonicUsec,
		TrapOID:               pdu.OID,
		Category:              "unknown",
		Severity:              "notice",
		SourceIP:              pdu.SourceIP,
		SourceUDPPeer:         pdu.PeerIP,
		PduType:               pdu.PduType,
		SnmpVersion:           pdu.Version,
		Varbinds:              pdu.Varbinds,
		Enrichment:            &model.TrapEnrichmentAudit{Source: pdu.SourceAudit},
	}

	if td != nil {
		entry.TrapName = td.Name
		entry.Category = model.Category(td.Category)
		entry.Severity = model.Severity(td.Severity)
		for i, vb := range entry.Varbinds {
			entry.Varbinds[i] = catalog.ResolveVarbind(vb.OID, vb, td)
		}
	}
	return entry
}

func renderEntryTemplates(entry *model.TrapEntry, td *catalog.TrapDef) {
	if entry == nil {
		return
	}
	if td != nil {
		entry.Message = catalog.RenderMessage(entry, td)
		entry.Labels = catalog.RenderLabels(entry, td)
		return
	}
	source := entry.SourceIP
	if source == "" {
		source = entry.SourceUDPPeer
	}
	entry.Message = fmt.Sprintf("SNMP trap %s from %s", entry.TrapOID, source)
}

func selectDedupKeyVarbinds(td *catalog.TrapDef, jobKeys []string) []string {
	if td != nil && len(td.DedupKeyVarbinds) > 0 {
		return td.DedupKeyVarbinds
	}
	return jobKeys
}

func newDeduper(
	jobName string,
	policy dedup.Policy,
	idx *catalog.Epoch,
	writer output.Writer,
	jobTelemetry *telemetry.Job,
	writeFailureDim telemetry.ErrorKind,
	monotonicNow func() int64,
) *dedup.Deduper {
	return dedup.New(policy, dedup.Options{
		MonotonicNow: monotonicNow,
		ResolveName: func(oid string) string {
			if idx != nil {
				if td := idx.Lookup(oid); td != nil && td.Name != "" {
					return td.Name
				}
			}
			return oid
		},
		OnSummary: func(summary dedup.Summary) {
			entry := &model.TrapEntry{
				JobName:               jobName,
				ReportType:            model.ReportTypeDedupSummary,
				ReceivedRealtimeUsec:  summary.ReceivedRealtimeUsec,
				ReceivedMonotonicUsec: summary.ReceivedMonotonicUsec,
				Message:               summary.Message,
				Severity:              "info",
				SummaryCounts:         summary.Counts,
			}
			if err := writer.Write(entry); err != nil {
				jobTelemetry.Error(writeFailureDim)
			}
		},
	})
}

func (j *Job) trapWriteFailureDim() telemetry.ErrorKind {
	if j.writeFailureDim != "" {
		return j.writeFailureDim
	}
	return writeFailureJournal
}
