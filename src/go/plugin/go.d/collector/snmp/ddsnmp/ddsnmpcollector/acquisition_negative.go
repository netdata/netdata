// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"slices"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// AcquisitionNegativeCause explains a permanent missing-OID entry without
// retaining unrelated values from the GET that established it.
type AcquisitionNegativeCause struct {
	ContextID  uint64           `json:"context_id"`
	Operation  uint64           `json:"operation"`
	ObservedAt time.Time        `json:"observed_at"`
	PDU        ddsnmp.SourcePDU `json:"pdu"`
}

type negativeEvidence struct {
	eligible map[string]bool
	causes   map[string]AcquisitionNegativeCause
}

// Only production suppression reads establish eligibility. Dynamic instance
// GETs bypass this map, so their churn cannot grow retained diagnostic history.
func isMissingOID(client snmputils.ScalarClient, missing map[string]bool, oid string) bool {
	if observer, ok := client.(*diagnosticClient); ok && observer.negative != nil && observer.SourceRecorder() != nil {
		if observer.negative.eligible == nil {
			observer.negative.eligible = make(map[string]bool)
		}
		observer.negative.eligible[oid] = true
	}
	return missing[oid]
}

func (c *diagnosticClient) recordMissing(pdu gosnmp.SnmpPDU) {
	source := c.SourceRecorder()
	if source == nil || c.negative == nil {
		return
	}
	oid := trimOID(pdu.Name)
	if !c.negative.eligible[oid] {
		return
	}
	if _, exists := c.negative.causes[oid]; exists {
		return
	}
	operation := source.Operation(source.Cursor())
	if operation == nil {
		return
	}
	if c.negative.causes == nil {
		c.negative.causes = make(map[string]AcquisitionNegativeCause)
	}
	c.negative.causes[oid] = AcquisitionNegativeCause{
		ContextID: operation.ContextID, Operation: operation.Ordinal, ObservedAt: operation.StartedAt,
		PDU: ddsnmp.SourcePDU{OID: strings.Clone(pdu.Name), Type: uint8(pdu.Type), Value: sourceValue(pdu.Value)},
	}
}

func (c *Collector) NegativeCauses() []AcquisitionNegativeCause {
	result := make([]AcquisitionNegativeCause, 0, len(c.negative.causes))
	for _, cause := range c.negative.causes {
		result = append(result, cause)
	}
	slices.SortFunc(result, func(a, b AcquisitionNegativeCause) int { return strings.Compare(a.PDU.OID, b.PDU.OID) })
	return result
}
