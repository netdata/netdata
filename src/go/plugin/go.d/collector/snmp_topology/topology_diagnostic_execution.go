// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

// Presence distinguishes recorded zero work from archives predating accounting.
// This allowlist is shared by the archive and selected-device inspection.
type topologyDiagnosticArchiveExecutionV1 struct {
	Preparation topologyDiagnosticArchivePreparationV1 `json:"preparation"`
	Walks       []topologyDiagnosticArchiveWalkV1      `json:"walks"`
}

type topologyDiagnosticArchivePreparationV1 struct {
	ElapsedNanos     int64 `json:"elapsed_ns"`
	GetRequests      int64 `json:"get_requests"`
	GetOIDs          int64 `json:"get_oids"`
	SNMPErrors       int64 `json:"snmp_errors"`
	MissingOIDs      int64 `json:"missing_oids"`
	ProcessingErrors int64 `json:"processing_errors"`
}

type topologyDiagnosticArchiveWalkV1 struct {
	RootOID      string `json:"root_oid"`
	ElapsedNanos int64  `json:"elapsed_ns"`
	Failed       bool   `json:"failed"`
}

func newTopologyDiagnosticArchiveExecutionV1(
	execution *ddsnmpcollector.AcquisitionExecutionReport,
) *topologyDiagnosticArchiveExecutionV1 {
	if execution == nil {
		return nil
	}
	p := execution.Preparation
	result := &topologyDiagnosticArchiveExecutionV1{
		Preparation: topologyDiagnosticArchivePreparationV1{
			ElapsedNanos:     int64(p.Elapsed),
			GetRequests:      p.GetRequests,
			GetOIDs:          p.GetOIDs,
			SNMPErrors:       p.SNMPErrors,
			MissingOIDs:      p.MissingOIDs,
			ProcessingErrors: p.ProcessingErrors,
		},
		Walks: make([]topologyDiagnosticArchiveWalkV1, 0, len(execution.Walks)),
	}
	for _, walk := range execution.Walks {
		result.Walks = append(result.Walks, topologyDiagnosticArchiveWalkV1{
			RootOID:      walk.RootOID,
			ElapsedNanos: int64(walk.Elapsed),
			Failed:       walk.Failed,
		})
	}
	return result
}

func (e *topologyDiagnosticArchiveExecutionV1) execution() (*ddsnmpcollector.AcquisitionExecutionReport, error) {
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
