// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"context"

	"github.com/netdata/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type tableWalkOutcome struct {
	sourceOperation uint64
	pdus            map[string]gosnmp.VarBind
	err             error
}

type tableWalkPass struct {
	execution  *AcquisitionExecutionReport
	outcomes   map[string]tableWalkOutcome
	walkedData map[string]map[string]gosnmp.VarBind
}

func newTableWalkPass(execution *AcquisitionExecutionReport) *tableWalkPass {
	return &tableWalkPass{
		execution:  execution,
		outcomes:   make(map[string]tableWalkOutcome),
		walkedData: make(map[string]map[string]gosnmp.VarBind),
	}
}

func walkTableWithStats(
	ctx context.Context,
	tc *tableCollector,
	oid string,
	stats *ddsnmp.CollectionStats,
	execution *AcquisitionExecutionReport,
) (map[string]gosnmp.VarBind, error) {
	pdus, err := tc.snmpWalk(ctx, oid, stats, execution)
	if err != nil {
		stats.Errors.SNMP++
		if !isPartialWalk(err) {
			return pdus, err
		}
	}
	stats.SNMP.TablesWalked++
	return pdus, err
}

func (p *tableWalkPass) walk(
	ctx context.Context,
	tc *tableCollector,
	oid string,
	stats *ddsnmp.CollectionStats,
) tableWalkOutcome {
	oid = trimOID(oid)
	if outcome, ok := p.outcomes[oid]; ok {
		return outcome
	}

	pdus, err := walkTableWithStats(ctx, tc, oid, stats, p.execution)
	outcome := tableWalkOutcome{pdus: pdus, err: err, sourceOperation: sourceRecorder(tc.snmpClient).Cursor()}
	p.outcomes[oid] = outcome
	if err != nil && !isPartialWalk(err) {
		return outcome
	}

	// A partial table feeds its dependents as a complete one does.
	p.walkedData[oid] = pdus
	return outcome
}
