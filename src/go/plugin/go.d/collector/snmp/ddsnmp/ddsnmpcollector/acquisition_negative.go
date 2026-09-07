// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// AcquisitionNegativeCause explains a permanent missing-OID entry without
// retaining unrelated values from the GET that established it.
type AcquisitionNegativeCause struct {
	ContextID  uint64
	Operation  uint64
	ObservedAt time.Time
	PDU        ddsnmp.SourcePDU
}

func (c *diagnosticClient) recordMissing(pdu gosnmp.SnmpPDU) {
	source := c.SourceRecorder()
	if source == nil || c.negativeCauses == nil {
		return
	}
	oid := trimOID(pdu.Name)
	if _, exists := (*c.negativeCauses)[oid]; exists {
		return
	}
	operation := source.Operation(source.Cursor())
	if operation == nil {
		return
	}
	if *c.negativeCauses == nil {
		*c.negativeCauses = make(map[string]AcquisitionNegativeCause)
	}
	(*c.negativeCauses)[oid] = AcquisitionNegativeCause{
		ContextID: operation.ContextID, Operation: operation.Ordinal, ObservedAt: operation.StartedAt,
		PDU: ddsnmp.SourcePDU{OID: strings.Clone(pdu.Name), Type: uint8(pdu.Type), Value: sourceValue(pdu.Value)},
	}
}

func (c *Collector) NegativeCauses() []AcquisitionNegativeCause {
	result := make([]AcquisitionNegativeCause, 0, len(c.negativeCauses))
	for _, cause := range c.negativeCauses {
		result = append(result, cause)
	}
	slices.SortFunc(result, func(a, b AcquisitionNegativeCause) int { return strings.Compare(a.PDU.OID, b.PDU.OID) })
	return result
}
