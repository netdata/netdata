// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
)

type DiagnosticArchive = topologydiag.DiagnosticArchive

type DiagnosticArchiveIdentity = topologydiag.DiagnosticArchiveIdentity

type DiagnosticDeviceInspection = topologydiag.DiagnosticDeviceInspection

type DiagnosticLinkInspection = topologydiag.DiagnosticLinkInspection

type DiagnosticLinkSubject = topologydiag.DiagnosticLinkSubject

type DiagnosticQueryOptions = topologydiag.DiagnosticQueryOptions

type DiagnosticSummary = topologydiag.DiagnosticSummary

type DiagnosticValidation = topologydiag.DiagnosticValidation

func DefaultDiagnosticQueryOptions() DiagnosticQueryOptions {
	return topologydiag.DefaultDiagnosticQueryOptions()
}

func InspectDiagnosticDocument(document snmpdiag.Document) (*DiagnosticArchive, error) {
	return topologydiag.InspectDiagnosticDocument(document, topologydiag.Semantics{
		ReplayAcquisition: replayTopologyAcquisitionEvidence,
		BuildGraph:        replayTopologyGraph,
	})
}
