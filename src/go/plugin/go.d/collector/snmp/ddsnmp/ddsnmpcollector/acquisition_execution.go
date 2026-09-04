// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import "time"

// AcquisitionExecutionReport records work charged to this profile, not to each
// configured route consuming it. Nil means execution accounting was not recorded.
type AcquisitionExecutionReport struct {
	Preparation AcquisitionPreparationStats
	Walks       []AcquisitionWalkReport
}

type AcquisitionPreparationStats struct {
	Elapsed          time.Duration
	GetRequests      int64
	GetOIDs          int64
	SNMPErrors       int64
	MissingOIDs      int64
	ProcessingErrors int64
}

// AcquisitionWalkReport measures one Handler call, including client retries but
// excluding later PDU-map/row processing. Failed does not classify terminal PDUs.
type AcquisitionWalkReport struct {
	RootOID string
	Elapsed time.Duration
	Failed  bool
}

func (c *acquisitionProfileCollection) executionReport() *AcquisitionExecutionReport {
	if c == nil {
		return nil
	}
	return &c.execution
}
