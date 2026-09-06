// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

func writeTopologyDiagnosticArchive(w io.Writer, diagnostics topologyDiagnostics) error {
	return writeTopologyDiagnosticArchiveWithProducerVersion(w, diagnostics, buildinfo.Version)
}

func writeTopologyDiagnosticArchiveWithProducerVersion(
	w io.Writer,
	diagnostics topologyDiagnostics,
	producerVersion string,
) error {
	if w == nil {
		return errors.New("write SNMP topology diagnostic archive: nil writer")
	}
	document, err := newTopologyDiagnosticArchiveDocumentV1(diagnostics, producerVersion)
	if err != nil {
		return fmt.Errorf("write SNMP topology diagnostic archive: %w", err)
	}
	return snmpdiag.Write(w, document)
}

func readTopologyDiagnosticArchive(r io.Reader, limits snmpdiag.ReadLimits) (topologyDiagnosticArchive, error) {
	document, err := snmpdiag.Read(r, limits)
	if err != nil {
		return topologyDiagnosticArchive{}, err
	}
	diagnostics, err := restoreArchiveDocument(document)
	if err != nil {
		return topologyDiagnosticArchive{}, fmt.Errorf("read SNMP topology diagnostic archive: %w", err)
	}
	return topologyDiagnosticArchive{
		producerVersion: document.Producer.AgentVersion,
		diagnostics:     diagnostics,
	}, nil
}

func newTopologyDiagnosticArchiveDocumentV1(
	diagnostics topologyDiagnostics,
	producerVersion string,
) (snmpdiag.Document, error) {
	snapshot, err := newTopologyDiagnosticArchiveSnapshotV1(diagnostics)
	if err != nil {
		return snmpdiag.Document{}, err
	}
	return snmpdiag.Document{
		Format:  snmpdiag.Format,
		Version: snmpdiag.Version,
		Producer: snmpdiag.Producer{
			AgentVersion: producerVersion,
		},
		Snapshot: snapshot,
	}, nil
}

// Native-cut tests inspect immutability and retention before wire conversion.
func (c *Collector) acquireTopologyDiagnostics() topologyDiagnostics {
	diagnostics, limits := captureTopologyCut(
		c.topologyRegistry,
		c.lastAbortedTopologyDiagnostic.Load(),
		c.currentTopologyDiagnosticGlobalLimits(),
	)
	diagnostics.lifecycle = acquireTopologyJobLifecycleCut(c.diagnosticProvider.source, limits)
	return diagnostics
}

func acquireTopologyJobLifecycleCut(
	source deviceLifecycleSource,
	limits topologyAcquisitionLimits,
) topologyJobLifecycleDiagnosticCut {
	projected := snmpdiag.CaptureLifecycle(source, limits.maxRecords, limits.maxLogicalBytes)
	result, err := restoreArchiveLifecycle(projected)
	if err != nil {
		return topologyJobLifecycleDiagnosticCut{
			state:  diagnosticCaptureUnavailable,
			reason: diagnosticCaptureReasonProjectionError,
		}
	}
	return result
}
