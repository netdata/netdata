// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologydiag"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func writeTopologyDiagnosticArchive(w io.Writer, diagnostics topologydiag.Cut) error {
	return writeTopologyDiagnosticArchiveWithProducerVersion(w, diagnostics, buildinfo.Version)
}

func writeTopologyDiagnosticArchiveWithProducerVersion(
	w io.Writer,
	diagnostics topologydiag.Cut,
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

func newTopologyDiagnosticArchiveDocumentV1(
	diagnostics topologydiag.Cut,
	producerVersion string,
) (snmpdiag.Document, error) {
	snapshot, err := topologydiag.NewSnapshot(diagnostics)
	if err != nil {
		return snmpdiag.Document{}, err
	}
	return snmpdiag.Document{
		Format:  snmpdiag.Format,
		Kind:    snmpdiag.KindTopology,
		Version: snmpdiag.Version,
		Producer: snmpdiag.Producer{
			AgentVersion: producerVersion,
		},
		Snapshot: snapshot,
	}, nil
}

// Native-cut tests inspect immutability and retention before wire conversion.
func (c *Collector) acquireTopologyDiagnostics() topologydiag.Cut {
	diagnostics := captureTopologyCut(
		c.topologyRegistry,
		c.lastAbortedTopologyDiagnostic.Load(),
	)
	return diagnostics
}

func openTestDiagnosticCut(tb testing.TB, cut topologydiag.Cut) *DiagnosticArchive {
	tb.Helper()
	document, err := newTopologyDiagnosticArchiveDocumentV1(cut, "v-test")
	require.NoError(tb, err)
	archive, err := InspectDiagnosticDocument(document)
	require.NoError(tb, err)
	return archive
}

func testDiagnosticQuery(options topologyoptions.QueryOptions) DiagnosticQueryOptions {
	depth := strconv.Itoa(options.Depth)
	if options.Depth == topologyoptions.DepthAllInternal {
		depth = topologyoptions.DepthAll
	}
	return DiagnosticQueryOptions{
		CollapseActorsByIP:     options.CollapseActorsByIP,
		EliminateNonIPInferred: options.EliminateNonIPInferred,
		MapType:                options.MapType,
		InferenceStrategy:      options.InferenceStrategy,
		ManagedDeviceFocus:     options.ManagedDeviceFocus,
		Depth:                  depth,
	}
}

func testLatestArchiveCapture(t testing.TB, document *snmpdiag.Document, index int) *snmpdiag.Capture {
	t.Helper()
	for i := range document.Snapshot.Topology.Devices[index].Captures {
		capture := &document.Snapshot.Topology.Devices[index].Captures[i]
		if slices.Contains(capture.Roles, "latest_attempt") {
			return capture
		}
	}
	t.Fatal("archive has no latest attempt")
	return nil
}
