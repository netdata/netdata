// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"

type ReportType = model.ReportType
type PduType = model.PduType
type SnmpVersion = model.SnmpVersion
type ASN1Type = model.ASN1Type
type Category = model.Category
type Severity = model.Severity
type VarbindValue = model.VarbindValue
type DedupSummary = model.DedupSummary
type DecodeErrorInfo = model.DecodeErrorInfo
type TrapSourceAudit = model.TrapSourceAudit
type TrapEnrichmentAudit = model.TrapEnrichmentAudit
type TrapEnrichmentLookup = model.TrapEnrichmentLookup
type TrapEntry = model.TrapEntry
type TrapPDU = model.TrapPDU

const (
	ReportTypeTrap         = model.ReportTypeTrap
	ReportTypeDedupSummary = model.ReportTypeDedupSummary
	ReportTypeDecodeError  = model.ReportTypeDecodeError
	PduTypeTrap            = model.PduTypeTrap
	PduTypeInform          = model.PduTypeInform
	SnmpVersionV1          = model.SnmpVersionV1
	SnmpVersionV2c         = model.SnmpVersionV2c
	SnmpVersionV3          = model.SnmpVersionV3
)
