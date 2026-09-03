// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type tableWalkOutcome struct {
	pdus map[string]gosnmp.SnmpPDU
	err  error
}

type tableWalkPass struct {
	execution  *AcquisitionExecutionReport
	outcomes   map[string]tableWalkOutcome
	walkedData map[string]map[string]gosnmp.SnmpPDU
}

func newTableWalkPass(execution *AcquisitionExecutionReport) *tableWalkPass {
	return &tableWalkPass{
		execution:  execution,
		outcomes:   make(map[string]tableWalkOutcome),
		walkedData: make(map[string]map[string]gosnmp.SnmpPDU),
	}
}

func walkTableWithStats(
	tc *tableCollector,
	oid string,
	stats *ddsnmp.CollectionStats,
	execution *AcquisitionExecutionReport,
) (map[string]gosnmp.SnmpPDU, error) {
	pdus, err := tc.snmpWalk(oid, stats, execution)
	if err != nil {
		stats.Errors.SNMP++
		return pdus, err
	}
	stats.SNMP.TablesWalked++
	return pdus, nil
}

func (p *tableWalkPass) walk(
	tc *tableCollector,
	oid string,
	stats *ddsnmp.CollectionStats,
) tableWalkOutcome {
	oid = trimOID(oid)
	if outcome, ok := p.outcomes[oid]; ok {
		return outcome
	}

	pdus, err := walkTableWithStats(tc, oid, stats, p.execution)
	outcome := tableWalkOutcome{pdus: pdus, err: err}
	p.outcomes[oid] = outcome
	if err != nil {
		return outcome
	}

	p.walkedData[oid] = pdus
	return outcome
}
