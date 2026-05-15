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

func TestTableRowProcessor_ExtractIndexPositionFromEnd(t *testing.T) {
	tests := map[string]struct {
		index    string
		position uint
		want     string
		wantOK   bool
	}{
		"last component": {
			index:    "1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1.2.1",
			position: 1,
			want:     "1",
			wantOK:   true,
		},
		"second from end": {
			index:    "1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1.2.1",
			position: 2,
			want:     "2",
			wantOK:   true,
		},
		"first component from full length": {
			index:    "7.8.9",
			position: 3,
			want:     "7",
			wantOK:   true,
		},
		"single component": {
			index:    "7",
			position: 1,
			want:     "7",
			wantOK:   true,
		},
		"position too large": {
			index:    "7.8.9",
			position: 4,
		},
		"zero position": {
			index: "7.8.9",
		},
		"empty index": {
			position: 1,
		},
	}

	p := newTableRowProcessor(logger.New())
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := p.extractIndexPositionFromEnd(tc.index, tc.position)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
