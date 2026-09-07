// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

// Presence distinguishes recorded zero work from archives predating accounting.
// This allowlist is shared by the archive and selected-device inspection.
func newTopologyDiagnosticArchiveExecutionV1(
	execution *ddsnmpcollector.AcquisitionExecutionReport,
) *snmpdiag.Execution {
	if execution == nil {
		return nil
	}
	p := execution.Preparation
	result := &snmpdiag.Execution{
		Preparation: snmpdiag.Preparation{
			ElapsedNanos:     int64(p.Elapsed),
			GetRequests:      p.GetRequests,
			GetOIDs:          p.GetOIDs,
			SNMPErrors:       p.SNMPErrors,
			MissingOIDs:      p.MissingOIDs,
			ProcessingErrors: p.ProcessingErrors,
		},
		Walks: make([]snmpdiag.Walk, 0, len(execution.Walks)),
	}
	for _, walk := range execution.Walks {
		result.Walks = append(result.Walks, snmpdiag.Walk{
			RootOID:      walk.RootOID,
			ElapsedNanos: int64(walk.Elapsed),
			Failed:       walk.Failed,
		})
	}
	return result
}

func restoreArchiveExecution(e *snmpdiag.Execution) (*ddsnmpcollector.AcquisitionExecutionReport, error) {
	if e == nil {
		return nil, nil
	}
	p := e.Preparation
	if p.ElapsedNanos < 0 || p.GetRequests < 0 || p.GetOIDs < 0 || p.SNMPErrors < 0 || p.MissingOIDs < 0 ||
		p.ProcessingErrors < 0 {
		return nil, errors.New("negative preparation measurement")
	}
	result := &ddsnmpcollector.AcquisitionExecutionReport{
		Preparation: ddsnmpcollector.AcquisitionPreparationStats{
			Elapsed:          time.Duration(p.ElapsedNanos),
			GetRequests:      p.GetRequests,
			GetOIDs:          p.GetOIDs,
			SNMPErrors:       p.SNMPErrors,
			MissingOIDs:      p.MissingOIDs,
			ProcessingErrors: p.ProcessingErrors,
		},
	}
	for _, walk := range e.Walks {
		if walk.ElapsedNanos < 0 || walk.RootOID == "" {
			return nil, errors.New("invalid walk measurement")
		}
		result.Walks = append(result.Walks, ddsnmpcollector.AcquisitionWalkReport{
			RootOID: walk.RootOID,
			Elapsed: time.Duration(walk.ElapsedNanos),
			Failed:  walk.Failed,
		})
	}
	return result, nil
}
