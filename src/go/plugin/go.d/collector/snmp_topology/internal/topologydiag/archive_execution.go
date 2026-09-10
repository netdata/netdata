// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

// Presence distinguishes recorded zero work from unobserved execution.
// This allowlist is shared by the archive and selected-device inspection.
func newArchiveExecutionV1(
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
		WalkOperations: execution.WalkOperations,
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
	result.WalkOperations = e.WalkOperations
	return result, nil
}
