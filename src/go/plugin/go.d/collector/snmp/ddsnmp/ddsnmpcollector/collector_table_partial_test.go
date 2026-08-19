// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestTableCollector_Collect_SuccessfulEmptyWalkIsPartialSuccess(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	profile := profileWithTwoTableMetrics()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", nil)
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", errors.New("timeout"))

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(0, 0), logger.New(), false)
	var stats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &stats)

	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(1), stats.SNMP.TablesWalked)
	assert.Equal(t, int64(2), stats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), stats.Errors.SNMP)
}

func TestTableCollector_Collect_AllWalksFailedStillErrors(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	profile := profileWithTwoTableMetrics()
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", errors.New("interface timeout"))
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", errors.New("IP timeout"))

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(0, 0), logger.New(), false)
	var stats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &stats)

	require.Error(t, err)
	assert.ErrorContains(t, err, "interface timeout")
	assert.ErrorContains(t, err, "IP timeout")
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), stats.SNMP.TablesWalked)
	assert.Equal(t, int64(2), stats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), stats.Errors.SNMP)
}

func TestTableCollector_Collect_PartialWalkWarningIsRateLimitedPerProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	profileA := profileWithTwoTableMetrics()
	profileB := profileWithTwoTableMetrics()
	profileB.SourceFile = "other-two-table-profile.yaml"
	for range 3 {
		expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", nil)
		expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", errors.New("timeout"))
	}

	var logs bytes.Buffer
	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(0, 0), logger.NewWithWriter(&logs), false)
	for _, profile := range []*ddsnmp.Profile{profileA, profileA, profileB} {
		var stats ddsnmp.CollectionStats
		metrics, err := collector.collect(profile, &stats)
		require.NoError(t, err)
		assert.Empty(t, metrics)
		assert.Equal(t, int64(1), stats.SNMP.TablesWalked)
		assert.Equal(t, int64(2), stats.SNMP.WalkRequests)
		assert.Equal(t, int64(1), stats.Errors.SNMP)
	}

	assert.Equal(t, 2, strings.Count(logs.String(), "failed to walk some SNMP tables"), logs.String())
}

func profileWithTwoTableMetrics() *ddsnmp.Profile {
	return createTestProfile("two-table-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.2.2", Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.4.20", Name: "ipAddrTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.4.20.1.2", Name: "ipAdEntIfIndex"}},
		},
	})
}
