// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
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

func TestTableCollector_Collect_CachedProcessingWarningIsRateLimitedPerProfile(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID  = "1.3.6.1.2.1.2.2"
		metricOID = "1.3.6.1.2.1.2.2.1.10.1"
	)
	metricsConfig := []ddprofiledefinition.MetricsConfig{{
		Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"}},
	}}
	profileA := createTestProfile("cached-processing-a.yaml", metricsConfig)
	profileB := createTestProfile("cached-processing-b.yaml", metricsConfig)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{createCounter32PDU(metricOID, 1)})
	for range 4 {
		expectSNMPGet(mockHandler, []string{metricOID}, []gosnmp.SnmpPDU{createStringPDU(metricOID, "not-a-number")})
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{createStringPDU(metricOID, "not-a-number")})
	}

	var logs bytes.Buffer
	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.NewWithWriter(&logs), false)
	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profileA, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	for _, profile := range []*ddsnmp.Profile{profileA, profileA, profileB, profileB} {
		var stats ddsnmp.CollectionStats
		metrics, err = collector.collect(profile, &stats)
		require.NoError(t, err)
		assert.Empty(t, metrics)
		assert.Equal(t, int64(1), stats.Errors.Processing.Table)
		assert.Equal(t, int64(0), stats.TableCache.Hits)
		assert.Equal(t, int64(1), stats.TableCache.Misses)
		assert.Equal(t, int64(1), stats.SNMP.GetRequests)
		assert.Equal(t, int64(1), stats.SNMP.WalkRequests)
	}

	assert.Equal(t, 2, strings.Count(logs.String(), "failed to collect 1 cached table metrics"), logs.String())
	assert.Contains(t, logs.String(), "cached-processing-a.yaml")
	assert.Contains(t, logs.String(), "ifInOctets")
	assert.Contains(t, logs.String(), "cached column 1.3.6.1.2.1.2.2.1.10 instance 1.3.6.1.2.1.2.2.1.10.1")
	assert.Contains(t, logs.String(), "returned 1.3.6.1.2.1.2.2.1.10.1 type OctetString")
	assert.NotContains(t, logs.String(), "not-a-number")
}

func TestTableCollector_Collect_EmptyTableIsNotCachedAsHit(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const tableOID = "1.3.6.1.2.1.2.2"
	profile := createTestProfile("empty-table-profile.yaml", []ddprofiledefinition.MetricsConfig{{
		Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"}},
	}})
	for range 2 {
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, nil)
	}

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	for range 2 {
		var stats ddsnmp.CollectionStats
		metrics, err := collector.collect(profile, &stats)
		require.NoError(t, err)
		assert.Empty(t, metrics)
		assert.Equal(t, int64(0), stats.TableCache.Hits)
		assert.Equal(t, int64(1), stats.TableCache.Misses)
		assert.Equal(t, int64(1), stats.SNMP.WalkRequests)
		assert.Equal(t, int64(1), stats.SNMP.TablesWalked)
	}
}

func TestTableCollector_Collect_EmptyTableInvalidatesStaleCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID   = "1.3.6.1.2.1.2.2"
		metricOID1 = "1.3.6.1.2.1.2.2.1.10.1"
		metricOID2 = "1.3.6.1.2.1.2.2.1.16.1"
	)
	profile := createTestProfile("dynamic-empty-table-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"}},
		},
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createCounter32PDU(metricOID1, 1),
		createCounter32PDU(metricOID2, 2),
	})
	mockHandler.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{}, nil).AnyTimes()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, nil)

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	var transitionStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &transitionStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), transitionStats.TableCache.Hits)
	assert.Equal(t, int64(2), transitionStats.TableCache.Misses)
	assert.Equal(t, int64(1), transitionStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.WalkRequests)
	require.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))
	require.False(t, cache.isConfigCached(profile.Definition.Metrics[1]))

	var emptyStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &emptyStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), emptyStats.TableCache.Hits)
	assert.Equal(t, int64(2), emptyStats.TableCache.Misses)
	assert.Equal(t, int64(0), emptyStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), emptyStats.SNMP.WalkRequests)
}

func TestTableCollector_Collect_TagOnlyRowsAreNotCached(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID = "1.3.6.1.2.1.4.20"
		tagOID   = "1.3.6.1.2.1.4.20.1.3.192.0.2.1"
	)
	profile := profileWithSameTableTag()
	for range 2 {
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
			createStringPDU(tagOID, "255.255.255.0"),
		})
	}

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)
	for range 2 {
		var stats ddsnmp.CollectionStats
		metrics, err := collector.collect(profile, &stats)
		require.NoError(t, err)
		assert.Empty(t, metrics)
		assert.Equal(t, int64(0), stats.Metrics.Rows)
		assert.Equal(t, int64(0), stats.TableCache.Hits)
		assert.Equal(t, int64(1), stats.TableCache.Misses)
		assert.Equal(t, int64(0), stats.SNMP.GetRequests)
		assert.Equal(t, int64(1), stats.SNMP.WalkRequests)
		assert.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))
	}
}

func TestTableCollector_Collect_TagOnlyRowsAreExcludedFromMixedTable(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const tableOID = "1.3.6.1.2.1.4.20"
	profile := profileWithSameTableTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.2.1.4.20.1.2.192.0.2.1", 17),
		createStringPDU("1.3.6.1.2.1.4.20.1.3.192.0.2.1", "255.255.255.0"),
		createStringPDU("1.3.6.1.2.1.4.20.1.3.192.0.2.2", "255.255.255.0"),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)
	var stats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &stats)

	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, map[string]string{"netmask": "255.255.255.0"}, metrics[0].Tags)
	assert.Equal(t, int64(1), stats.Metrics.Rows)
	cachedOIDs, cachedTags, ok := cache.getCachedData(profile.Definition.Metrics[0])
	require.True(t, ok)
	assert.Len(t, cachedOIDs, 1)
	assert.Len(t, cachedTags, 1)
	assert.Contains(t, cachedOIDs, "192.0.2.1")
	assert.NotContains(t, cachedOIDs, "192.0.2.2")
}

func TestTableCollector_Collect_TagOnlyTransitionInvalidatesStaleCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID  = "1.3.6.1.2.1.4.20"
		metricOID = "1.3.6.1.2.1.4.20.1.2.192.0.2.1"
		tagOID    = "1.3.6.1.2.1.4.20.1.3.192.0.2.1"
	)
	profile := profileWithSameTableTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID, 17),
		createStringPDU(tagOID, "255.255.255.0"),
	})
	expectSNMPGet(mockHandler, []string{metricOID}, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createStringPDU(tagOID, "255.255.255.0"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createStringPDU(tagOID, "255.255.255.0"),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	var transitionStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &transitionStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), transitionStats.Metrics.Rows)
	assert.Equal(t, int64(0), transitionStats.TableCache.Hits)
	assert.Equal(t, int64(1), transitionStats.TableCache.Misses)
	assert.Equal(t, int64(1), transitionStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.WalkRequests)
	assert.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))

	var tagOnlyStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &tagOnlyStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), tagOnlyStats.TableCache.Hits)
	assert.Equal(t, int64(1), tagOnlyStats.TableCache.Misses)
	assert.Equal(t, int64(0), tagOnlyStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), tagOnlyStats.SNMP.WalkRequests)
}

func TestTableCollector_Collect_PartialCachedResponseFallsBackAndPrunesRows(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID   = "1.3.6.1.2.1.4.20"
		metricOID1 = "1.3.6.1.2.1.4.20.1.2.192.0.2.1"
		metricOID2 = "1.3.6.1.2.1.4.20.1.2.192.0.2.2"
		tagOID1    = "1.3.6.1.2.1.4.20.1.3.192.0.2.1"
		tagOID2    = "1.3.6.1.2.1.4.20.1.3.192.0.2.2"
	)
	profile := profileWithSameTableTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID1, 17),
		createGauge32PDU(metricOID2, 18),
		createStringPDU(tagOID1, "255.255.255.0"),
		createStringPDU(tagOID2, "255.255.255.0"),
	})
	expectSNMPGet(mockHandler, []string{metricOID1, metricOID2}, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID1, 19),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID1, 20),
		createStringPDU(tagOID1, "255.255.255.128"),
	})
	expectSNMPGet(mockHandler, []string{metricOID1}, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID1, 21),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	var transitionStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &transitionStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 20, metrics[0].Value)
	assert.Equal(t, map[string]string{"netmask": "255.255.255.128"}, metrics[0].Tags)
	assert.Equal(t, int64(1), transitionStats.Metrics.Rows)
	assert.Equal(t, int64(0), transitionStats.TableCache.Hits)
	assert.Equal(t, int64(1), transitionStats.TableCache.Misses)
	assert.Equal(t, int64(1), transitionStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.TablesWalked)
	assert.Equal(t, int64(0), transitionStats.SNMP.TablesCached)
	cachedOIDs, _, ok := cache.getCachedData(profile.Definition.Metrics[0])
	require.True(t, ok)
	assert.Len(t, cachedOIDs, 1)
	assert.Contains(t, cachedOIDs, "192.0.2.1")
	assert.NotContains(t, cachedOIDs, "192.0.2.2")

	var cachedStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &cachedStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 21, metrics[0].Value)
	assert.Equal(t, map[string]string{"netmask": "255.255.255.128"}, metrics[0].Tags)
	assert.Equal(t, int64(1), cachedStats.Metrics.Rows)
	assert.Equal(t, int64(1), cachedStats.TableCache.Hits)
	assert.Equal(t, int64(0), cachedStats.TableCache.Misses)
	assert.Equal(t, int64(1), cachedStats.SNMP.GetRequests)
	assert.Equal(t, int64(0), cachedStats.SNMP.WalkRequests)
}

func TestTableCollector_Collect_EmptySourceFetchesCachedDependencyWhenItBecomesActive(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID   = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		sourceMetricOID  = sourceTableOID + ".1.1.1"
		dependencyOID    = "1.3.6.1.2.1.47.1.1.1.1.2"
		dependencyRowOID = dependencyOID + ".1"
	)
	profile := profileWithCrossTableOnlyTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyRowOID, "Power Supply 1"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 110),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyRowOID, "Power Supply 1"),
	})
	expectSNMPGet(mockHandler, []string{sourceMetricOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 120),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var emptyStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &emptyStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(2), emptyStats.TableCache.Misses)
	assert.Equal(t, int64(2), emptyStats.SNMP.WalkRequests)
	assert.True(t, cache.isConfigCached(profile.Definition.Metrics[1]))

	var activeStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &activeStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, map[string]string{"ent_descr": "Power Supply 1"}, metrics[0].Tags)
	assert.Equal(t, int64(0), activeStats.TableCache.Hits)
	assert.Equal(t, int64(2), activeStats.TableCache.Misses)
	assert.Equal(t, int64(2), activeStats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), activeStats.SNMP.TablesWalked)
	assert.Equal(t, int64(0), activeStats.SNMP.TablesCached)

	var cachedStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &cachedStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 120, metrics[0].Value)
	assert.Equal(t, map[string]string{"ent_descr": "Power Supply 1"}, metrics[0].Tags)
	assert.Equal(t, int64(2), cachedStats.TableCache.Hits)
	assert.Equal(t, int64(0), cachedStats.TableCache.Misses)
	assert.Equal(t, int64(1), cachedStats.SNMP.GetRequests)
	assert.Equal(t, int64(0), cachedStats.SNMP.WalkRequests)
}

func TestTableCollector_Collect_EmptySourceKeepsCachedDependencyWithoutRewalking(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		dependencyOID  = "1.3.6.1.2.1.47.1.1.1.1.2"
	)
	profile := profileWithCrossTableOnlyTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyOID+".1", "Power Supply 1"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)

	var emptyStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &emptyStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)
	assert.Equal(t, int64(1), emptyStats.TableCache.Hits)
	assert.Equal(t, int64(1), emptyStats.TableCache.Misses)
	assert.Equal(t, int64(1), emptyStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), emptyStats.SNMP.TablesWalked)
	assert.Equal(t, int64(1), emptyStats.SNMP.TablesCached)
	assert.True(t, cache.isConfigCached(profile.Definition.Metrics[1]))
}

func TestTableCollector_Collect_FailedOnDemandDependencyDoesNotCacheIncompleteTags(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID   = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		sourceMetricOID  = sourceTableOID + ".1.1.1"
		dependencyOID    = "1.3.6.1.2.1.47.1.1.1.1.2"
		dependencyRowOID = dependencyOID + ".1"
	)
	profile := profileWithCrossTableOnlyTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyRowOID, "Power Supply 1"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 110),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, dependencyOID, errors.New("timeout"))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 120),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyRowOID, "Power Supply 1"),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)

	var failedStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &failedStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Empty(t, metrics[0].Tags)
	assert.Equal(t, int64(1), failedStats.Errors.SNMP)
	assert.Equal(t, int64(0), failedStats.TableCache.Hits)
	assert.Equal(t, int64(2), failedStats.TableCache.Misses)
	assert.Equal(t, int64(2), failedStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), failedStats.SNMP.TablesWalked)
	assert.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))
	assert.False(t, cache.isConfigCached(profile.Definition.Metrics[1]))

	var recoveredStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &recoveredStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 120, metrics[0].Value)
	assert.Equal(t, map[string]string{"ent_descr": "Power Supply 1"}, metrics[0].Tags)
	assert.True(t, cache.isConfigCached(profile.Definition.Metrics[0]))
}

func TestTableCollector_Collect_ColdFailedDependencyDoesNotCacheIncompleteTags(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID   = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		sourceMetricOID  = sourceTableOID + ".1.1.1"
		dependencyOID    = "1.3.6.1.2.1.47.1.1.1.1.2"
		dependencyRowOID = dependencyOID + ".1"
	)
	profile := profileWithCrossTableOnlyTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 110),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, dependencyOID, errors.New("timeout"))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 120),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyRowOID, "Power Supply 1"),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var failedStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &failedStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Empty(t, metrics[0].Tags)
	assert.Equal(t, int64(1), failedStats.Errors.SNMP)
	assert.Equal(t, int64(2), failedStats.TableCache.Misses)
	assert.Equal(t, int64(2), failedStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), failedStats.SNMP.TablesWalked)
	assert.False(t, cache.isConfigCached(profile.Definition.Metrics[0]))
	assert.False(t, cache.isConfigCached(profile.Definition.Metrics[1]))

	var recoveredStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &recoveredStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 120, metrics[0].Value)
	assert.Equal(t, map[string]string{"ent_descr": "Power Supply 1"}, metrics[0].Tags)
	assert.Equal(t, int64(0), recoveredStats.Errors.SNMP)
	assert.Equal(t, int64(2), recoveredStats.TableCache.Misses)
	assert.Equal(t, int64(2), recoveredStats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), recoveredStats.SNMP.TablesWalked)
	assert.True(t, cache.isConfigCached(profile.Definition.Metrics[0]))
	assert.True(t, cache.isConfigCached(profile.Definition.Metrics[1]))
}

func TestTableCollector_Collect_OnDemandDependencyIsWalkedOncePerCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID = "1.3.6.1.4.1.25461.1.1.7.1.2.1"
		dependencyOID  = "1.3.6.1.2.1.47.1.1.1.1.2"
	)
	profile := profileWithCrossTableOnlyTag()
	secondSource := profile.Definition.Metrics[0]
	secondSource.Symbols = []ddprofiledefinition.SymbolConfig{{
		OID:  sourceTableOID + ".1.2",
		Name: "panEntryFRUModuleCurrent",
	}}
	profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{
		profile.Definition.Metrics[0],
		secondSource,
		profile.Definition.Metrics[1],
	}

	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyOID+".1", "Power Supply 1"),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceTableOID+".1.1.1", 110),
		createGauge32PDU(sourceTableOID+".1.2.1", 12),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyOID+".1", "Power Supply 1"),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)

	var activeStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &activeStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	for _, metric := range metrics {
		assert.Equal(t, map[string]string{"ent_descr": "Power Supply 1"}, metric.Tags)
	}
	assert.Equal(t, int64(0), activeStats.TableCache.Hits)
	assert.Equal(t, int64(3), activeStats.TableCache.Misses)
	assert.Equal(t, int64(2), activeStats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), activeStats.SNMP.TablesWalked)
}

func TestTableCollector_Collect_FreshSymbolDependencyIsAuthoritativeForAllConfigs(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID      = "1.3.6.1.4.1.99999.1"
		sourceMetricOID     = sourceTableOID + ".1.1"
		dependencyTableOID  = "1.3.6.1.4.1.99999.2"
		dependencyMetricOID = dependencyTableOID + ".1.1"
	)
	profile := profileWithSymbolBearingCrossTableTag()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(sourceMetricOID, 110),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(dependencyMetricOID, 20),
	})

	cache := newTableCache(time.Hour, 0)
	collector := newTableCollector(mockHandler, make(map[string]bool), cache, logger.New(), false)

	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	var activeStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &activeStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	for _, metric := range metrics {
		switch metric.Name {
		case "sourceValue":
			assert.EqualValues(t, 110, metric.Value)
			assert.Equal(t, map[string]string{"dependency_value": "20"}, metric.Tags)
		case "dependencyValue":
			assert.EqualValues(t, 20, metric.Value)
		default:
			t.Errorf("unexpected metric %q", metric.Name)
		}
	}
	assert.Equal(t, int64(0), activeStats.TableCache.Hits)
	assert.Equal(t, int64(2), activeStats.TableCache.Misses)
	assert.Equal(t, int64(0), activeStats.SNMP.GetRequests)
	assert.Equal(t, int64(2), activeStats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), activeStats.SNMP.TablesWalked)
	assert.Equal(t, int64(0), activeStats.SNMP.TablesCached)
}

func TestTableCollector_Collect_DependencyRefreshSettlesReverseChainOnly(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableAOID = "1.3.6.1.4.1.99999.1"
		tableBOID = "1.3.6.1.4.1.99999.2"
		tableCOID = "1.3.6.1.4.1.99999.3"
		tableDOID = "1.3.6.1.4.1.99999.4"
	)
	metricOID := func(tableOID string) string { return tableOID + ".1.1" }
	profile := createTestProfile("dependency-chain-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableAOID, Name: "tableA"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableAOID + ".1", Name: "valueA"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:   "value_b",
				Table: "tableB",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  tableBOID + ".1",
					Name: "valueB",
				},
			}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableBOID, Name: "tableB"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableBOID + ".1", Name: "valueB"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:   "value_c",
				Table: "tableC",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  tableCOID + ".1",
					Name: "valueC",
				},
			}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableCOID, Name: "tableC"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableCOID + ".1", Name: "valueC"}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableDOID, Name: "tableD"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableDOID + ".1", Name: "valueD"}},
		},
	})

	for tableOID, value := range map[string]uint{
		tableAOID: 10,
		tableBOID: 20,
		tableCOID: 30,
		tableDOID: 40,
	} {
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
			createGauge32PDU(metricOID(tableOID), value),
		})
	}

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 4)

	expectSNMPGet(mockHandler, []string{metricOID(tableAOID)}, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID(tableAOID), 11),
	})
	expectSNMPGet(mockHandler, []string{metricOID(tableBOID)}, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID(tableBOID), 21),
	})
	expectSNMPGet(mockHandler, []string{metricOID(tableCOID)}, nil)
	expectSNMPGet(mockHandler, []string{metricOID(tableDOID)}, []gosnmp.SnmpPDU{
		createGauge32PDU(metricOID(tableDOID), 41),
	})
	for tableOID, value := range map[string]uint{
		tableAOID: 100,
		tableBOID: 200,
		tableCOID: 300,
	} {
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
			createGauge32PDU(metricOID(tableOID), value),
		})
	}

	var transitionStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &transitionStats)
	require.NoError(t, err)
	require.Len(t, metrics, 4)
	byName := make(map[string]ddsnmp.Metric, len(metrics))
	for _, metric := range metrics {
		byName[metric.Name] = metric
	}
	assert.EqualValues(t, 100, byName["valueA"].Value)
	assert.Equal(t, map[string]string{"value_b": "200"}, byName["valueA"].Tags)
	assert.EqualValues(t, 200, byName["valueB"].Value)
	assert.Equal(t, map[string]string{"value_c": "300"}, byName["valueB"].Tags)
	assert.EqualValues(t, 300, byName["valueC"].Value)
	assert.EqualValues(t, 41, byName["valueD"].Value)
	assert.Equal(t, int64(1), transitionStats.TableCache.Hits)
	assert.Equal(t, int64(3), transitionStats.TableCache.Misses)
	assert.Equal(t, int64(4), transitionStats.SNMP.GetRequests)
	assert.Equal(t, int64(3), transitionStats.SNMP.WalkRequests)
	assert.Equal(t, int64(3), transitionStats.SNMP.TablesWalked)
	assert.Equal(t, int64(1), transitionStats.SNMP.TablesCached)
}

func TestTableCollector_Collect_ResolvesWholeTableBeforeEmittingCachedConfigs(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID   = "1.3.6.1.2.1.2.2"
		metricOID1 = tableOID + ".1.10.1"
		metricOID2 = tableOID + ".1.16.1"
	)
	profile := createTestProfile("whole-table-route-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1.10", Name: "ifInOctets"}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1.16", Name: "ifOutOctets"}},
		},
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createCounter32PDU(metricOID1, 10),
		createCounter32PDU(metricOID2, 20),
	})
	expectSNMPGet(mockHandler, []string{metricOID1}, []gosnmp.SnmpPDU{
		createCounter32PDU(metricOID1, 11),
	})
	expectSNMPGet(mockHandler, []string{metricOID2}, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createCounter32PDU(metricOID1, 12),
		createCounter32PDU(metricOID2, 22),
	})

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	var transitionStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &transitionStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	values := make(map[string]int64, len(metrics))
	for _, metric := range metrics {
		values[metric.Name] = metric.Value
	}
	assert.Equal(t, map[string]int64{"ifInOctets": 12, "ifOutOctets": 22}, values)
	assert.Equal(t, int64(0), transitionStats.TableCache.Hits)
	assert.Equal(t, int64(2), transitionStats.TableCache.Misses)
	assert.Equal(t, int64(2), transitionStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), transitionStats.SNMP.TablesWalked)
	assert.Equal(t, int64(0), transitionStats.SNMP.TablesCached)
}

func TestTableCollector_Collect_FailedFallbackIsOneTableMiss(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tableOID   = "1.3.6.1.2.1.2.2"
		metricOID1 = tableOID + ".1.10.1"
		metricOID2 = tableOID + ".1.16.1"
	)
	profile := createTestProfile("failed-fallback-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1.10", Name: "ifInOctets"}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1.16", Name: "ifOutOctets"}},
		},
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createCounter32PDU(metricOID1, 10),
		createCounter32PDU(metricOID2, 20),
	})
	expectSNMPGet(mockHandler, []string{metricOID1}, nil)
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, tableOID, errors.New("fallback timeout"))

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	var failedStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &failedStats)
	require.Error(t, err)
	assert.ErrorContains(t, err, "fallback timeout")
	assert.Empty(t, metrics)
	assert.Equal(t, int64(0), failedStats.TableCache.Hits)
	assert.Equal(t, int64(2), failedStats.TableCache.Misses)
	assert.Equal(t, int64(1), failedStats.SNMP.GetRequests)
	assert.Equal(t, int64(1), failedStats.SNMP.WalkRequests)
	assert.Equal(t, int64(1), failedStats.Errors.SNMP)
}

func TestTableCollector_Collect_CachedAuxiliaryDoesNotMaskAllFailedSources(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		sourceTableOID1 = "1.3.6.1.4.1.99999.1"
		sourceTableOID2 = "1.3.6.1.4.1.99999.2"
		dependencyOID   = "1.3.6.1.4.1.99999.3.1"
	)
	profile := profileWithTwoSourcesAndAuxiliary()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID1, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, sourceTableOID2, nil)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependencyOID, []gosnmp.SnmpPDU{
		createStringPDU(dependencyOID+".1", "dependency"),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, sourceTableOID1, errors.New("source one timeout"))
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, sourceTableOID2, errors.New("source two timeout"))

	collector := newTableCollector(mockHandler, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	var initialStats ddsnmp.CollectionStats
	metrics, err := collector.collect(profile, &initialStats)
	require.NoError(t, err)
	assert.Empty(t, metrics)

	var failedStats ddsnmp.CollectionStats
	metrics, err = collector.collect(profile, &failedStats)
	require.Error(t, err)
	assert.ErrorContains(t, err, "source one timeout")
	assert.ErrorContains(t, err, "source two timeout")
	assert.Empty(t, metrics)
	assert.Equal(t, int64(1), failedStats.TableCache.Hits)
	assert.Equal(t, int64(2), failedStats.TableCache.Misses)
	assert.Equal(t, int64(2), failedStats.SNMP.WalkRequests)
	assert.Equal(t, int64(2), failedStats.Errors.SNMP)
}

func profileWithSameTableTag() *ddsnmp.Profile {
	return createTestProfile("same-table-tag-profile.yaml", []ddprofiledefinition.MetricsConfig{{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.4.20",
			Name: "ipAddrTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{
			OID:  "1.3.6.1.2.1.4.20.1.2",
			Name: "ipAdEntIfIndex",
		}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag: "netmask",
			Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID:  "1.3.6.1.2.1.4.20.1.3",
				Name: "ipAdEntNetMask",
			},
		}},
	}})
}

func profileWithCrossTableOnlyTag() *ddsnmp.Profile {
	profile := createTestProfile("cross-table-transition-profile.yaml", []ddprofiledefinition.MetricsConfig{{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1",
			Name: "panEntityFRUModuleTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{
			OID:  "1.3.6.1.4.1.25461.1.1.7.1.2.1.1.1",
			Name: "panEntryFRUModulePowerUsed",
		}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag:   "ent_descr",
			Table: "entPhysicalTable",
			Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID:  "1.3.6.1.2.1.47.1.1.1.1.2",
				Name: "entPhysicalDescr",
			},
		}},
	}})
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
	return profile
}

func profileWithSymbolBearingCrossTableTag() *ddsnmp.Profile {
	return createTestProfile("symbol-bearing-cross-table-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  "1.3.6.1.4.1.99999.1",
				Name: "sourceTable",
			},
			Symbols: []ddprofiledefinition.SymbolConfig{{
				OID:  "1.3.6.1.4.1.99999.1.1",
				Name: "sourceValue",
			}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:   "dependency_value",
				Table: "dependencyTable",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  "1.3.6.1.4.1.99999.2.1",
					Name: "dependencyValue",
				},
			}},
		},
		{
			Table: ddprofiledefinition.SymbolConfig{
				OID:  "1.3.6.1.4.1.99999.2",
				Name: "dependencyTable",
			},
			Symbols: []ddprofiledefinition.SymbolConfig{{
				OID:  "1.3.6.1.4.1.99999.2.1",
				Name: "dependencyValue",
			}},
		},
	})
}

func profileWithTwoSourcesAndAuxiliary() *ddsnmp.Profile {
	profile := createTestProfile("two-source-auxiliary-profile.yaml", []ddprofiledefinition.MetricsConfig{
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.1", Name: "sourceTableOne"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.4.1.99999.1.1", Name: "sourceOneValue"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:   "dependency_value",
				Table: "dependencyTable",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  "1.3.6.1.4.1.99999.3.1",
					Name: "dependencyValue",
				},
			}},
		},
		{
			Table:   ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.2", Name: "sourceTableTwo"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: "1.3.6.1.4.1.99999.2.1", Name: "sourceTwoValue"}},
			MetricTags: []ddprofiledefinition.MetricTagConfig{{
				Tag:   "dependency_value",
				Table: "dependencyTable",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  "1.3.6.1.4.1.99999.3.1",
					Name: "dependencyValue",
				},
			}},
		},
	})
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
	return profile
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
