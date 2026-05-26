// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

type ReportType string

const (
	ReportTypeTrap               ReportType = "trap"
	ReportTypeDedupSummary       ReportType = "deduplication_summary"
	ReportTypeDecodeErrorSummary ReportType = "decode_error_summary"
)

type PduType string

const (
	PduTypeTrap   PduType = "trap"
	PduTypeInform PduType = "inform"
)

type SnmpVersion string

const (
	SnmpVersionV1  SnmpVersion = "v1"
	SnmpVersionV2c SnmpVersion = "v2c"
	SnmpVersionV3  SnmpVersion = "v3"
)

type ASN1Type string

type Category string

type Severity string

type VarbindValue struct {
	Name  string   `json:"name,omitempty"`
	OID   string   `json:"oid"`
	Type  ASN1Type `json:"type"`
	Value any      `json:"value"`
	Enum  string   `json:"enum,omitempty"`
}

type DedupSummary struct {
	TotalSuppressed int64            `json:"total_suppressed"`
	PeriodSec       int64            `json:"period_sec"`
	Fingerprints    int64            `json:"fingerprints"`
	ByTrap          map[string]int64 `json:"by_trap"`
}

type TrapEntry struct {
	JobName               string
	ReportType            ReportType
	ReceivedRealtimeUsec  int64
	ReceivedMonotonicUsec int64
	TrapOID               string
	TrapName              string
	Category              Category
	Severity              Severity
	Message               string
	SourceIP              string
	SourceUDPPeer         string
	DeviceHostname        string
	DeviceVendor          string
	PduType               PduType
	SnmpVersion           SnmpVersion
	SourceVnodeID         string
	TopologyInterface     string
	TopologyNeighbors     string
	Labels                map[string]string
	Varbinds              []VarbindValue
	SummaryCounts         *DedupSummary
}
