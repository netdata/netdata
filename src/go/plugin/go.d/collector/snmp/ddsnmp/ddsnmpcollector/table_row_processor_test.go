// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableRowProcessor_ProcessRowMetrics_SkipsTextDateNoValue(t *testing.T) {
	p := newTableRowProcessor(logger.New())

	row := &tableRowData{
		pdus: map[string]gosnmp.SnmpPDU{
			"1.3.6.1.4.1.999.1.1.1": createStringPDU("1.3.6.1.4.1.999.1.1.1.1", "never"),
		},
		tags:       map[string]string{},
		staticTags: map[string]string{},
		tableName:  "licenseTable",
	}
	ctx := &tableRowProcessingContext{
		columnOIDs: map[string][]ddprofiledefinition.SymbolConfig{
			"1.3.6.1.4.1.999.1.1.1": {
				{
					OID:    "1.3.6.1.4.1.999.1.1.1",
					Name:   "license.expiry",
					Format: "text_date",
				},
			},
		},
	}

	metrics, err := p.processRowMetrics(row, ctx)

	require.NoError(t, err)
	assert.Empty(t, metrics)
}
