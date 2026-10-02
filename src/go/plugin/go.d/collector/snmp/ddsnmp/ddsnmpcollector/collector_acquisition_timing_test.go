// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_AcquisitionScalarFailureTiming(t *testing.T) {
	const oid = "1.3.6.1.4.1.99999.1.0"
	for name, topology := range map[string]bool{"ordinary": false, "topology": true} {
		t.Run(name, func(t *testing.T) {
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			handler.EXPECT().Get([]string{oid}).DoAndReturn(func([]string) (*gosnmp.SnmpPacket, error) {
				time.Sleep(time.Millisecond)
				return nil, errors.New("transport failure")
			})
			profile := createTestProfile("scalar.yaml", nil)
			metric := createScalarMetric(oid, "identity")
			if topology {
				profile.Definition.Topology = []ddprofiledefinition.TopologyConfig{
					{Kind: ddsnmp.KindIfName, MetricsConfig: metric},
				}
			} else {
				profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{metric}
			}
			var report AcquisitionProfileReport
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(
					func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = value },
				),
			})
			_, err := collector.Collect()
			require.Error(t, err)
			assert.GreaterOrEqual(t, report.Stats.Timing.Scalar, time.Millisecond)
		})
	}
}

func TestCollector_AcquisitionPreparationBatchesFailureAndRetry(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	profile := createTestProfile("preparation.yaml", nil)
	var oids []string
	var pdus []gosnmp.SnmpPDU
	for i := range 11 {
		oid := fmt.Sprintf("1.3.6.1.4.1.99999.%02d.0", i)
		oids = append(oids, oid)
		pdus = append(pdus, createStringPDU(oid, "value"))
		profile.Definition.MetricTags = append(profile.Definition.MetricTags, ddprofiledefinition.GlobalMetricTagConfig{
			MetricTagConfig: ddprofiledefinition.MetricTagConfig{
				Tag: fmt.Sprintf("tag%d", i),
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID:  oid,
					Name: "tag",
				},
			},
		})
	}
	// A second tag consumes the first OID without requesting it twice.
	profile.Definition.MetricTags = append(profile.Definition.MetricTags, profile.Definition.MetricTags[0])
	pdus[0] = createNoSuchObjectPDU(oids[0])
	expectSNMPGet(handler, oids[:10], pdus[:10])
	expectSNMPGetError(handler, oids[10:], errors.New("second batch failed"))
	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: new(SourceRecorder).Wrap(handler),
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(
			func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r },
		),
	})
	_, err := collector.Collect()
	require.Error(t, err)
	require.NotNil(t, report.Execution)
	firstReport := report
	p := report.Execution.Preparation
	assert.EqualValues(t, 2, p.GetRequests)
	assert.EqualValues(t, 11, p.GetOIDs)
	assert.EqualValues(t, 1, p.SNMPErrors)
	assert.EqualValues(t, 1, p.MissingOIDs)
	assert.Equal(t, p.Elapsed, report.Stats.Timing.Preparation)
	assert.Positive(t, p.Elapsed)
	assert.Zero(t, report.Stats.Timing.Scalar)
	assert.Empty(t, testWalkSources(t, collector, report.Execution))

	// A failed preparation retries; the already-missing OID stays suppressed.
	expectSNMPGet(handler, oids[1:], pdus[1:])
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.EqualValues(t, 1, metrics[0].Stats.SNMP.GetRequests)
	assert.EqualValues(t, 10, metrics[0].Stats.SNMP.GetOIDs)
	assert.Zero(t, metrics[0].Stats.Errors.SNMP)
	assert.Positive(t, metrics[0].Stats.Errors.MissingOIDs)
	metrics, err = collector.Collect()
	require.NoError(t, err)
	assert.Zero(t, metrics[0].Stats.SNMP.GetRequests, "initialized inputs are cached")
	assert.Equal(t, p, firstReport.Execution.Preparation, "later collections must not mutate retained evidence")
	assert.Zero(t, report.Execution.Preparation.GetRequests)
}

func TestCollector_AcquisitionPreparationNonfatalMetadataErrors(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	const base = "1.3.6.1.4.1.99999"
	profile := createTestProfile("metadata.yaml", nil)
	profile.Definition.SysobjectIDMetadata = []ddprofiledefinition.SysobjectIDMetadataEntryConfig{{
		SysobjectID: base,
		Metadata: map[string]ddprofiledefinition.MetadataField{
			"model": {Symbol: ddprofiledefinition.SymbolConfig{
				OID: base + ".1.0",
			}},
		},
	}}
	profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
		"device": {Fields: map[string]ddprofiledefinition.MetadataField{
			"vendor": {Value: "synthetic"},
			"model": {Symbols: []ddprofiledefinition.SymbolConfig{
				{OID: base + ".2.0", Format: "uint32"},
				{OID: base + ".3.0"},
			}},
		}},
	}
	profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{createScalarMetric(base+".4.0", "value")}
	expectSNMPGetError(handler, []string{base + ".1.0"}, errors.New("ignored rule failure"))
	expectSNMPGet(handler, []string{base + ".2.0", base + ".3.0"}, []gosnmp.SnmpPDU{
		createStringPDU(base+".2.0", "not matching"), createStringPDU(base+".3.0", "fallback"),
	})
	expectSNMPGet(handler, []string{base + ".4.0"}, []gosnmp.SnmpPDU{createGauge32PDU(base+".4.0", 10)})
	var report AcquisitionProfileReport
	collector := New(
		Config{
			SnmpClient:  handler,
			Profiles:    []*ddsnmp.Profile{profile},
			SysObjectID: base,
			Log:         logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(
				func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r },
			),
		},
	)
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "fallback", metrics[0].DeviceMetadata["model"].Value)
	assert.EqualValues(t, 2, report.Execution.Preparation.GetRequests)
	assert.EqualValues(t, 3, report.Execution.Preparation.GetOIDs)
	assert.EqualValues(t, 1, report.Execution.Preparation.SNMPErrors)
	assert.EqualValues(t, 1, report.Execution.Preparation.ProcessingErrors)
	assert.EqualValues(t, 3, report.Stats.SNMP.GetRequests, "scalar work is separate from preparation")
	assert.EqualValues(t, 4, report.Stats.SNMP.GetOIDs)
	assert.EqualValues(t, 1, report.Stats.Errors.Processing.Preparation)
}

func TestCollector_AcquisitionSharedWalkAccounting(t *testing.T) {
	for name, tc := range map[string]struct {
		version gosnmp.SnmpVersion
		failed  bool
	}{
		"v1 success": {version: gosnmp.Version1},
		"v1 failure": {version: gosnmp.Version1, failed: true},
		"v2 success": {version: gosnmp.Version2c},
		"v2 failure": {version: gosnmp.Version2c, failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			version, failed := tc.version, tc.failed
			ctrl, handler := setupMockHandler(t)
			defer ctrl.Finish()
			const root = "1.3.6.1.4.1.99999.1"
			metric := ddprofiledefinition.MetricsConfig{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  root,
					Name: "table",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}},
			}
			var callErr error
			if failed {
				callErr = errors.New("walk failure")
			}
			handler.EXPECT().Version().Return(version)
			walk := func(string) ([]gosnmp.SnmpPDU, error) { time.Sleep(time.Millisecond); return nil, callErr }
			if version == gosnmp.Version1 {
				handler.EXPECT().WalkAll(root).DoAndReturn(walk)
			} else {
				handler.EXPECT().BulkWalkAll(root).DoAndReturn(walk)
			}
			var reports []AcquisitionProfileReport
			collector := New(Config{
				SnmpClient: new(SourceRecorder).Wrap(handler),
				Log:        logger.New(),
				Profiles: []*ddsnmp.Profile{
					createTestProfile("a.yaml", []ddprofiledefinition.MetricsConfig{metric}),
					createTestProfile("b.yaml", []ddprofiledefinition.MetricsConfig{metric}),
				},
				AcquisitionObserver: AcquisitionObserverFunc(func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { reports = append(reports, r) }),
			})
			_, err := collector.Collect()
			assert.Equal(t, failed, err != nil)
			require.Len(t, reports, 2)
			walks := testWalkSources(t, collector, reports[0].Execution)
			require.Len(t, walks, 1)
			assert.Empty(t, testWalkSources(t, collector, reports[1].Execution), "shared consumer must not duplicate work")
			recorded := walks[0]
			assert.Equal(t, root, recorded.RequestedOIDs[0])
			assert.Equal(t, failed, recorded.Failure.Reason != "")
			assert.GreaterOrEqual(t, time.Duration(recorded.ElapsedNanos), time.Millisecond)
			assert.EqualValues(t, 1, reports[0].Stats.SNMP.WalkRequests)
			assert.Zero(t, reports[1].Stats.SNMP.WalkRequests)
		})
	}
}

func TestCollector_AcquisitionBGPWalkPassAccounting(t *testing.T) {
	ctrl, handler := setupMockHandler(t)
	defer ctrl.Finish()
	const anchor = "1.3.6.1.4.1.99999.70.1"
	const dependency = "1.3.6.1.4.1.99999.70.2"
	pdus := []gosnmp.SnmpPDU{createGauge32PDU(anchor+".1.4.192.0.2.1.1.1", 42)}
	// The regular-table session and the BGP pass execute this root separately.
	expectSNMPWalk(handler, gosnmp.Version2c, anchor, pdus)
	expectSNMPWalk(handler, gosnmp.Version2c, anchor, pdus)
	expectSNMPWalkError(handler, gosnmp.Version2c, dependency, errors.New("dependency unavailable"))
	profile := createTestProfile("passes.yaml", []ddprofiledefinition.MetricsConfig{{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  anchor,
			Name: "anchor",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: anchor + ".1", Name: "value"}},
	}})
	profile.Definition.BGP = []ddprofiledefinition.BGPConfig{crossTableBGPTestConfig(), crossTableBGPTestConfig()}
	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: new(SourceRecorder).Wrap(handler),
		Log:        logger.New(),
		Profiles:   []*ddsnmp.Profile{profile},
		AcquisitionObserver: AcquisitionObserverFunc(
			func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r },
		),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Error(t, metrics[0].BGPCollectError)
	walks := testWalkSources(t, collector, report.Execution)
	require.Len(t, walks, 3, "memoized BGP anchor and failed dependency must not add executions")
	var roots []string
	for _, walk := range walks {
		roots = append(roots, walk.RequestedOIDs[0])
	}
	assert.Equal(t, []string{anchor, anchor, dependency}, roots)
	assert.Empty(t, walks[0].Failure.Reason)
	assert.Empty(t, walks[1].Failure.Reason)
	assert.NotEmpty(t, walks[2].Failure.Reason)
	assert.EqualValues(t, 3, report.Stats.SNMP.WalkRequests)
	assert.EqualValues(t, 1, report.Stats.Errors.SNMP)
}
