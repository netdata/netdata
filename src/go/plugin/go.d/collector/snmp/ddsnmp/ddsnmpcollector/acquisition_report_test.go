// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_AcquisitionObserverReportsEveryProfileInExecutionOrder(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const failedTagOID = "1.3.6.1.4.1.99999.1.0"
	failed := &ddsnmp.Profile{
		SourceFile: "/private/catalog/a-failed.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag:    "site",
					Symbol: ddprofiledefinition.SymbolConfigCompat{OID: failedTagOID, Name: "site"},
				},
			}},
		},
	}
	succeeded := createTestProfile("/private/catalog/b-succeeded.yaml", nil)
	expectSNMPGetError(mockHandler, []string{failedTagOID}, errors.New("secret transport detail"))

	type observation struct {
		report     AcquisitionProfileReport
		hasMetrics bool
	}
	var got []observation
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{succeeded, failed},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, metrics *ddsnmp.ProfileMetrics) {
			got = append(got, observation{report: report, hasMetrics: metrics != nil})
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, got, 2)

	assert.Equal(t, uint32(0), got[0].report.Identity.Ordinal)
	assert.Equal(t, AcquisitionProfileOutcomeFailed, got[0].report.Outcome)
	assert.Equal(t, AcquisitionFailurePhasePrepare, got[0].report.FailurePhase)
	assert.True(t, got[0].hasMetrics)
	require.Len(t, got[0].report.Routes, 1)
	assert.Equal(t, AcquisitionRouteKindProfileTagScalar, got[0].report.Routes[0].Kind)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, got[0].report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionFailureClassTransport, got[0].report.Routes[0].FailureClass)

	assert.Equal(t, uint32(1), got[1].report.Identity.Ordinal)
	assert.Equal(t, AcquisitionProfileOutcomeSuccess, got[1].report.Outcome)
	assert.Equal(t, AcquisitionFailurePhaseNone, got[1].report.FailurePhase)
	assert.True(t, got[1].hasMetrics)

	for _, observation := range got {
		assert.NotZero(t, observation.report.Identity.RouteDigest)
		assert.NotContains(t, observation.report.String(), "/private/catalog")
		assert.NotContains(t, observation.report.String(), "secret transport detail")
	}
}

func TestCollector_AcquisitionObserverReportsProfileTagAndMetadataRoutesFromGETAndCache(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tagOID      = "1.3.6.1.4.1.99999.10.1.0"
		metadataOID = "1.3.6.1.4.1.99999.10.2.0"
	)
	profile := &ddsnmp.Profile{
		SourceFile: "profile-inputs.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag:    "site",
					Symbol: ddprofiledefinition.SymbolConfigCompat{OID: tagOID, Name: "site"},
				},
			}},
			Metadata: ddprofiledefinition.MetadataConfig{
				ddprofiledefinition.MetadataDeviceResource: {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: metadataOID, Name: "serialNumber"}},
					},
				},
			},
		},
	}
	expectSNMPGet(mockHandler, []string{tagOID}, []gosnmp.SnmpPDU{createStringPDU(tagOID, "lab")})
	expectSNMPGet(mockHandler, []string{metadataOID}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(metadataOID)})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, report)
		}),
	})
	for range 2 {
		metrics, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, metrics, 1)
		assert.Equal(t, "lab", metrics[0].Tags["site"])
		assert.NotContains(t, metrics[0].DeviceMetadata, "serial_number")
	}
	require.Len(t, reports, 2)

	first := acquisitionRoutesByKind(reports[0].Routes)
	require.Contains(t, first, AcquisitionRouteKindProfileTagScalar)
	require.Contains(t, first, AcquisitionRouteKindMetadataScalar)
	assert.Equal(t, AcquisitionRouteSourceGET, first[AcquisitionRouteKindProfileTagScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeValues, first[AcquisitionRouteKindProfileTagScalar].Outcome)
	assert.Equal(t, AcquisitionRouteSourceGET, first[AcquisitionRouteKindMetadataScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeMissing, first[AcquisitionRouteKindMetadataScalar].Outcome)

	second := acquisitionRoutesByKind(reports[1].Routes)
	require.Contains(t, second, AcquisitionRouteKindProfileTagScalar)
	require.Contains(t, second, AcquisitionRouteKindMetadataScalar)
	assert.Equal(t, AcquisitionRouteSourceCache, second[AcquisitionRouteKindProfileTagScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeValues, second[AcquisitionRouteKindProfileTagScalar].Outcome)
	assert.Equal(t, AcquisitionRouteSourceCache, second[AcquisitionRouteKindMetadataScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeMissing, second[AcquisitionRouteKindMetadataScalar].Outcome)
}

func TestCollector_AcquisitionObserverReportsLaterRoutesAsNotObservedAfterPrepareFailure(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		tagOID      = "1.3.6.1.4.1.99999.11.1.0"
		metadataOID = "1.3.6.1.4.1.99999.11.2.0"
	)
	profile := &ddsnmp.Profile{
		SourceFile: "prepare-failure.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag:    "site",
					Symbol: ddprofiledefinition.SymbolConfigCompat{OID: tagOID, Name: "site"},
				},
			}},
			Metadata: ddprofiledefinition.MetadataConfig{
				ddprofiledefinition.MetadataDeviceResource: {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"serial_number": {Symbol: ddprofiledefinition.SymbolConfig{OID: metadataOID, Name: "serialNumber"}},
					},
				},
			},
		},
	}
	expectSNMPGetError(mockHandler, []string{tagOID}, errors.New("tag transport failure"))

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	_, err := collector.Collect()
	require.Error(t, err)

	require.Len(t, report.Routes, 2)
	routes := acquisitionRoutesByKind(report.Routes)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, routes[AcquisitionRouteKindProfileTagScalar].Outcome)
	assert.Equal(t, AcquisitionRouteOutcomeNotObserved, routes[AcquisitionRouteKindMetadataScalar].Outcome)
}

func TestCollector_AcquisitionObserverReportsPatternTagValue(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const tagOID = "1.3.6.1.4.1.99999.12.1.0"
	profile := &ddsnmp.Profile{
		SourceFile: "pattern-tag.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Symbol:  ddprofiledefinition.SymbolConfigCompat{OID: tagOID, Name: "sysName"},
					Pattern: mustCompileRegex(`(.*)-(.*)`),
					Tags:    map[string]string{"site": "$1", "role": "$2"},
				},
			}},
		},
	}
	expectSNMPGet(mockHandler, []string{tagOID}, []gosnmp.SnmpPDU{createStringPDU(tagOID, "lab-core")})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, value)
		}),
	})
	for range 2 {
		metrics, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, metrics, 1)
		assert.Equal(t, "lab", metrics[0].Tags["site"])
		assert.Equal(t, "core", metrics[0].Tags["role"])
	}
	require.Len(t, reports, 2)
	for _, report := range reports {
		require.Len(t, report.Routes, 1)
		assert.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[0].Outcome)
	}
	assert.Equal(t, AcquisitionRouteSourceGET, reports[0].Routes[0].Source)
	assert.Equal(t, AcquisitionRouteSourceCache, reports[1].Routes[0].Source)
}

func TestCollector_AcquisitionObserverPanicDoesNotChangeCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{createTestProfile("profile.yaml", nil)},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {
			panic("observer failure")
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
}

func TestCollector_AcquisitionProfileDigestUsesRoutesNotSourcePath(t *testing.T) {
	const (
		tableOID  = "1.3.6.1.4.1.99999.2"
		columnOID = tableOID + ".1.1"
	)

	collectDigest := func(t *testing.T, source, oid string) [32]byte {
		t.Helper()
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()
		expectSNMPWalk(mockHandler, gosnmp.Version2c, oid, nil)

		profile := &ddsnmp.Profile{
			SourceFile: source,
			Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindArpEntry,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: oid, Name: "arpTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: columnOID, Name: "arpEntry"}},
				},
			}}},
		}

		var reports []AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				reports = append(reports, report)
			}),
		})
		_, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, reports, 1)
		return reports[0].Identity.RouteDigest
	}

	digestA := collectDigest(t, "/first/private/path/profile.yaml", tableOID)
	digestB := collectDigest(t, "/different/private/path/profile.yaml", tableOID)
	digestChangedRoute := collectDigest(t, "/first/private/path/profile.yaml", tableOID+".9")

	assert.Equal(t, digestA, digestB)
	assert.NotEqual(t, digestA, digestChangedRoute)
}

func TestCollector_AcquisitionObserverReportsPartialTableCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	profile := profileWithTwoTableMetrics()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.2.2", nil)
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.4.20", errors.New("private transport detail"))

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, report)
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, reports, 1)
	require.Len(t, reports[0].Routes, 2)
	assert.Equal(t, AcquisitionProfileOutcomePartial, reports[0].Outcome)
	assert.Equal(t, AcquisitionRouteOutcomeEmpty, reports[0].Routes[0].Outcome)
	assert.Equal(t, AcquisitionRouteSourceWalk, reports[0].Routes[0].Source)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, reports[0].Routes[1].Outcome)
	assert.Equal(t, AcquisitionFailureClassTransport, reports[0].Routes[1].FailureClass)
	assert.NotContains(t, reports[0].String(), "private transport detail")
}

func TestCollector_AcquisitionObserverReportsSyntheticDependencyAndTagFailure(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	dependency, source := crossTableDependencyTestConfigs("1.3.6.1.4.1.99999.50")
	profile := &ddsnmp.Profile{
		SourceFile: "topology-dependency.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddsnmp.KindArpEntry,
			MetricsConfig: source,
		}}},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)
	var dependencyRouteOID string
	for _, cfg := range profile.Definition.Topology {
		if cfg.Table.Name == dependency.Table.Name && len(cfg.Symbols) == 0 {
			dependencyRouteOID = cfg.Table.OID
			break
		}
	}
	require.NotEmpty(t, dependencyRouteOID)

	expectSNMPWalk(mockHandler, gosnmp.Version2c, source.Table.OID, []gosnmp.SnmpPDU{
		createGauge32PDU(source.Symbols[0].OID+".1", 10),
	})
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, dependencyRouteOID, errors.New("dependency timeout"))

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].TopologyMetrics, 1)
	require.Empty(t, metrics[0].TopologyMetrics[0].Tags)

	require.Len(t, report.Routes, 2)
	routes := make(map[string]AcquisitionRouteReport, len(report.Routes))
	for _, route := range report.Routes {
		routes[route.RootOID] = route
	}
	assert.Equal(t, AcquisitionProfileOutcomePartial, report.Outcome)
	assert.Equal(t, AcquisitionRouteOutcomePartial, routes[source.Table.OID].Outcome)
	assert.Equal(t, AcquisitionFailureClassDependency, routes[source.Table.OID].FailureClass)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, routes[dependencyRouteOID].Outcome)
	assert.Equal(t, AcquisitionFailureClassTransport, routes[dependencyRouteOID].FailureClass)
}

func TestCollector_AcquisitionObserverReportsSyntheticDependencyVarbindCounts(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const oidBase = "1.3.6.1.4.1.99999.55"
	dependency, source := crossTableDependencyTestConfigs(oidBase)
	source.MetricTags = append(source.MetricTags, ddprofiledefinition.MetricTagConfig{
		Tag:   "second_dependency_value",
		Table: dependency.Table.Name,
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID:  oidBase + ".2.2",
			Name: "secondDependencyValue",
		},
	})
	profile := &ddsnmp.Profile{
		SourceFile: "topology-dependency-counts.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddsnmp.KindArpEntry,
			MetricsConfig: source,
		}}},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(profile)

	expectSNMPWalk(mockHandler, gosnmp.Version2c, source.Table.OID, []gosnmp.SnmpPDU{
		createGauge32PDU(source.Symbols[0].OID+".1", 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependency.Table.OID, []gosnmp.SnmpPDU{
		createGauge32PDU(oidBase+".2.1.1", 20),
		createGauge32PDU(oidBase+".2.2.1", 30),
		createGauge32PDU(oidBase+".2.1.2", 40),
		createGauge32PDU(oidBase+".2.2.2", 50),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	_, err := collector.Collect()
	require.NoError(t, err)

	require.Len(t, report.Routes, 2)
	routes := make(map[string]AcquisitionRouteReport, len(report.Routes))
	for _, route := range report.Routes {
		routes[route.RootOID] = route
	}
	assert.Equal(t, AcquisitionRouteOutcomeValues, routes[dependency.Table.OID].Outcome)
	assert.Zero(t, routes[dependency.Table.OID].Rows)
	assert.Equal(t, uint64(4), routes[dependency.Table.OID].Values)
}

func TestCollector_AcquisitionObserverFiltersSharedWalkToSyntheticDependencyRoot(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const oidBase = "1.3.6.1.4.1.99999.56"
	dependency, source := crossTableDependencyTestConfigs(oidBase)
	owner := createTestProfile("a-dependency-owner.yaml", []ddprofiledefinition.MetricsConfig{{
		Table: dependency.Table,
		Symbols: []ddprofiledefinition.SymbolConfig{{
			OID:  oidBase + ".2.9",
			Name: "unrelatedValue",
		}},
	}})
	topology := &ddsnmp.Profile{
		SourceFile: "b-topology-dependency.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddsnmp.KindArpEntry,
			MetricsConfig: source,
		}}},
	}
	ddsnmp.HandleCrossTableTagsWithoutMetrics(topology)

	expectSNMPWalk(mockHandler, gosnmp.Version2c, source.Table.OID, []gosnmp.SnmpPDU{
		createGauge32PDU(source.Symbols[0].OID+".1", 10),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, dependency.Table.OID, []gosnmp.SnmpPDU{
		createGauge32PDU(oidBase+".2.9.1", 20),
	})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{topology, owner},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, report)
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Len(t, reports, 2)

	logicalDependencyRoot := oidBase + ".2.1"
	var dependencyRoute *AcquisitionRouteReport
	for i := range reports[1].Routes {
		if reports[1].Routes[i].RootOID == logicalDependencyRoot {
			dependencyRoute = &reports[1].Routes[i]
			break
		}
	}
	require.NotNil(t, dependencyRoute)
	assert.Equal(t, AcquisitionRouteOutcomeEmpty, dependencyRoute.Outcome)
	assert.Zero(t, dependencyRoute.Rows)
	assert.Zero(t, dependencyRoute.Values)
}

func TestCollector_AcquisitionReportLimitFailsOpen(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const tableOID = "1.3.6.1.4.1.99999.57"
	profile := &ddsnmp.Profile{
		SourceFile: "acquisition-report-limit.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind: ddsnmp.KindIfName,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifNameTable"},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1", Name: "ifName"}},
			},
		}}},
	}
	expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{
		createStringPDU(tableOID+".1.1", "eth0"),
		createStringPDU(tableOID+".1.2", "eth1"),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
		AcquisitionReportLimits: AcquisitionReportLimits{
			MaxRecords:      3,
			MaxLogicalBytes: 1 << 20,
		},
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].TopologyMetrics, 2, "diagnostic exhaustion must not change live collection")
	assert.Equal(t, AcquisitionReportStateLimitExceeded, report.State)
	assert.Empty(t, report.Routes)
	assert.Empty(t, report.TopologyValueReferences)
	assert.Equal(t, uint64(3), collector.acquisitionBudget.records)
	assert.Equal(t, AcquisitionReportLimitRecords, collector.acquisitionBudget.limit)
}

func TestCollector_AcquisitionReportLogicalByteLimitFailsOpen(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const scalarOID = "1.3.6.1.4.1.99999.58.0"
	profile := &ddsnmp.Profile{
		SourceFile: "acquisition-report-byte-limit.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind: ddsnmp.KindIfName,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Symbol: ddprofiledefinition.SymbolConfig{OID: scalarOID, Name: "sysName"},
			},
		}}},
	}
	expectSNMPGet(mockHandler, []string{scalarOID}, []gosnmp.SnmpPDU{createIntegerPDU(scalarOID, 1)})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
		AcquisitionReportLimits: AcquisitionReportLimits{
			MaxRecords:      100,
			MaxLogicalBytes: 1,
		},
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].TopologyMetrics, 1, "diagnostic exhaustion must not change live collection")
	assert.Equal(t, AcquisitionReportStateLimitExceeded, report.State)
	assert.Equal(t, AcquisitionReportLimitLogicalBytes, report.Limit)
	assert.Empty(t, report.Routes)
}

func TestAcquisitionProfileCollectionLimitStopsDiagnosticConstruction(t *testing.T) {
	const (
		tagOID      = "1.3.6.1.4.1.99999.59.1.0"
		metadataOID = "1.3.6.1.4.1.99999.59.2"
		tableOID    = "1.3.6.1.4.1.99999.59.3"
	)
	metadataFields := map[string]ddprofiledefinition.MetadataField{
		"serial_number": {Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: metadataOID + ".1", Name: "serialPrimary"},
			{OID: metadataOID + ".2", Name: "serialFallback"},
		}},
	}
	profile := &ddsnmp.Profile{
		SourceFile: "limit-construction-barrier.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{{
				MetricTagConfig: ddprofiledefinition.MetricTagConfig{
					Tag:    "site",
					Symbol: ddprofiledefinition.SymbolConfigCompat{OID: tagOID, Name: "site"},
				},
			}},
			Metadata: ddprofiledefinition.MetadataConfig{
				ddprofiledefinition.MetadataDeviceResource: {Fields: metadataFields},
			},
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindIfName,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "ifNameTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: tableOID + ".1", Name: "ifName"}},
				},
			}},
		},
	}
	records, logicalBytes := acquisitionProfileCollectionShape(profile, "")

	t.Run("initial precharge failure", func(t *testing.T) {
		budget := newAcquisitionReportBudget(AcquisitionReportLimits{
			MaxRecords:      records - 1,
			MaxLogicalBytes: logicalBytes,
		})
		collection := newAcquisitionProfileCollection(buildAcquisitionProfilePlan(profile, 0), profile, "", &budget)
		require.True(t, collection.reportLimited())

		assert.Nil(t, collection.metricScalarObserver())
		assert.Nil(t, collection.topologyScalarObserver())
		assert.Nil(t, collection.metricTableScope())
		assert.Nil(t, collection.topologyTableScope())
		assert.Nil(t, collection.globalTagObserver(profile.Definition.MetricTags))
		assert.Nil(t, collection.metadataObserver(metadataFields))
		assert.Equal(t, -1, collection.addRoute(AcquisitionRouteKindTopologyScalar, tagOID))
		assert.Empty(t, collection.routes)
		assert.Empty(t, collection.tableBindings)
	})

	t.Run("dynamic exhaustion", func(t *testing.T) {
		budget := newAcquisitionReportBudget(AcquisitionReportLimits{
			MaxRecords:      records,
			MaxLogicalBytes: logicalBytes + 16,
		})
		collection := newAcquisitionProfileCollection(buildAcquisitionProfilePlan(profile, 0), profile, "", &budget)
		require.False(t, collection.reportLimited())
		tableScope := collection.topologyTableScope()
		require.NotNil(t, tableScope)
		assert.Nil(t, collection.metadataObserver(map[string]ddprofiledefinition.MetadataField{
			"vendor": {Value: "static"},
		}))
		assert.Zero(t, collection.metadataCursor)
		metadataObserver := collection.metadataObserver(metadataFields)
		require.NotNil(t, metadataObserver)
		metadataObserver.start(map[string]bool{
			metadataOID + ".1": true,
			metadataOID + ".2": true,
		})
		metadataRoute := metadataObserver.route("serial_number")
		require.NotNil(t, metadataRoute)
		assert.Equal(t, AcquisitionRouteOutcomeMissing, metadataRoute.Outcome)
		routeCount := len(collection.routes)
		metadataCursor := collection.metadataCursor

		collection.addTopologyValueReference(AcquisitionValueReference{})
		require.True(t, collection.reportLimited())

		assert.Nil(t, collection.metricScalarObserver())
		assert.Nil(t, collection.topologyScalarObserver())
		assert.Nil(t, collection.metricTableScope())
		assert.Nil(t, collection.topologyTableScope())
		assert.Nil(t, collection.globalTagObserver(profile.Definition.MetricTags))
		assert.Nil(t, collection.metadataObserver(metadataFields))
		assert.Equal(t, metadataCursor, collection.metadataCursor)
		assert.Equal(t, -1, collection.addRoute(AcquisitionRouteKindTopologyScalar, tagOID))
		assert.Nil(t, tableScope.bind(0, &tableCollectionRequest{}))
		assert.Len(t, collection.routes, routeCount)
		assert.Empty(t, collection.tableBindings)
	})
}

func TestCollector_AcquisitionReportLimitRecoversAfterTransientCollectExhaustion(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		topologyOID = "1.3.6.1.4.1.99999.59.10.0"
		metricOID   = "1.3.6.1.4.1.99999.59.11.0"
	)
	topologyProfile := &ddsnmp.Profile{
		SourceFile: "a-transient-topology.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind: ddsnmp.KindIfName,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Symbol: ddprofiledefinition.SymbolConfig{OID: topologyOID, Name: "sysName"},
			},
		}}},
	}
	metricProfile := createTestProfile("b-regular-metric.yaml", []ddprofiledefinition.MetricsConfig{{
		Symbol: ddprofiledefinition.SymbolConfig{OID: metricOID, Name: "sysUpTime"},
	}})

	for range 2 {
		expectSNMPGet(mockHandler, []string{topologyOID}, []gosnmp.SnmpPDU{createIntegerPDU(topologyOID, 1)})
		expectSNMPGet(mockHandler, []string{metricOID}, []gosnmp.SnmpPDU{createIntegerPDU(metricOID, 2)})
	}
	expectSNMPGet(mockHandler, []string{topologyOID}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(topologyOID)})
	expectSNMPGet(mockHandler, []string{metricOID}, []gosnmp.SnmpPDU{createIntegerPDU(metricOID, 2)})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{topologyProfile, metricProfile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, report)
		}),
		AcquisitionReportLimits: AcquisitionReportLimits{MaxRecords: 100, MaxLogicalBytes: 1 << 20},
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Len(t, reports, 2)
	assert.Equal(t, AcquisitionReportStateAvailable, reports[0].State)
	assert.Equal(t, AcquisitionReportStateAvailable, reports[1].State)

	collector.acquisitionReportLimits.MaxRecords = 4
	metrics, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Len(t, reports, 4)
	assert.Equal(t, AcquisitionReportStateLimitExceeded, reports[2].State)
	assert.Equal(t, AcquisitionReportStateLimitExceeded, reports[3].State)

	metrics, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Len(t, reports, 6)
	assert.Empty(t, metrics[0].TopologyMetrics)
	assert.Equal(t, AcquisitionReportStateAvailable, reports[4].State)
	assert.Equal(t, AcquisitionReportStateAvailable, reports[5].State)
	assert.Equal(t, AcquisitionReportLimitNone, collector.acquisitionCacheLimit)
}

func TestCollector_AcquisitionObserverDoesNotReportUnprocessedFreshTableAsEmpty(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		regularTableOID  = "1.3.6.1.4.1.99999.60"
		topologyTableOID = "1.3.6.1.4.1.99999.61"
	)
	profile := &ddsnmp.Profile{
		SourceFile: "unprocessed-topology.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{{
				Table:   ddprofiledefinition.SymbolConfig{OID: regularTableOID, Name: "regularTable"},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: regularTableOID + ".1", Name: "regularValue"}},
			}},
			Topology: []ddprofiledefinition.TopologyConfig{{
				Kind: ddsnmp.KindArpEntry,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: topologyTableOID, Name: "topologyTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: topologyTableOID + ".1", Name: "topologyValue"}},
				},
			}},
		},
	}
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, regularTableOID, errors.New("regular timeout"))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, topologyTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(topologyTableOID+".1.1", 1),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	_, err := collector.Collect()
	require.Error(t, err)

	require.Len(t, report.Routes, 2)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionRouteSourceWalk, report.Routes[1].Source)
	assert.Equal(t, AcquisitionRouteOutcomeNotObserved, report.Routes[1].Outcome)
}

func TestCollector_AcquisitionObserverAssociatesTopologyValuesWithRoutes(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		firstTableOID  = "1.3.6.1.4.1.99999.70"
		secondTableOID = "1.3.6.1.4.1.99999.71"
	)
	profile := &ddsnmp.Profile{
		SourceFile: "value-references.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{
			{
				Kind: ddsnmp.KindArpEntry,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: firstTableOID, Name: "firstTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: firstTableOID + ".1", Name: "firstValue"}},
				},
			},
			{
				Kind: ddsnmp.KindFdbEntry,
				MetricsConfig: ddprofiledefinition.MetricsConfig{
					Table:   ddprofiledefinition.SymbolConfig{OID: secondTableOID, Name: "secondTable"},
					Symbols: []ddprofiledefinition.SymbolConfig{{OID: secondTableOID + ".1", Name: "secondValue"}},
				},
			},
		}},
	}
	expectSNMPWalk(mockHandler, gosnmp.Version2c, firstTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(firstTableOID+".1.7", 1),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, secondTableOID, []gosnmp.SnmpPDU{
		createGauge32PDU(secondTableOID+".1.9", 1),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].TopologyMetrics, 2)
	require.Len(t, report.TopologyValueReferences, 2)
	assert.Equal(t, AcquisitionValueReference{RouteOrdinal: 0, RowOrdinal: 0, ValueOrdinal: 0},
		report.TopologyValueReferences[0])
	assert.Equal(t, AcquisitionValueReference{RouteOrdinal: 1, RowOrdinal: 0, ValueOrdinal: 0},
		report.TopologyValueReferences[1])
}

func TestCollector_AcquisitionObserverDistinguishesCurrentAndInheritedMissingSources(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const oid = "1.3.6.1.4.1.99999.80.0"
	first := createTestProfile("a-current.yaml", []ddprofiledefinition.MetricsConfig{{
		Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "currentMissing"},
	}})
	second := createTestProfile("b-inherited.yaml", []ddprofiledefinition.MetricsConfig{{
		Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "inheritedMissing"},
	}})
	expectSNMPGet(mockHandler, []string{oid}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(oid)})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{second, first},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, value)
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Len(t, reports, 2)
	assert.Equal(t, AcquisitionRouteSourceGET, reports[0].Routes[0].Source)
	assert.Equal(t, AcquisitionRouteOutcomeMissing, reports[0].Routes[0].Outcome)
	assert.Equal(t, AcquisitionRouteSourceCache, reports[1].Routes[0].Source)
	assert.Equal(t, AcquisitionRouteOutcomeMissing, reports[1].Routes[0].Outcome)
}

func TestCollector_AcquisitionObserverReportsScalarsDiscoveredMissingInCurrentGET(t *testing.T) {
	t.Run("metric scalar", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		const oid = "1.3.6.1.4.1.99999.40.1.0"
		profile := createTestProfile("metric-scalar.yaml", []ddprofiledefinition.MetricsConfig{{
			Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "missingMetric"},
		}})
		expectSNMPGet(mockHandler, []string{oid}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(oid)})

		var report AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				report = value
			}),
		})
		_, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, report.Routes, 1)
		assert.Equal(t, AcquisitionRouteOutcomeMissing, report.Routes[0].Outcome)
		assert.Zero(t, report.Routes[0].Rejected)
	})

	t.Run("BGP scalar", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		oids := []string{
			"1.3.6.1.4.1.99999.20.1.0",
			"1.3.6.1.4.1.99999.20.2.0",
		}
		expectSNMPGet(mockHandler, oids, []gosnmp.SnmpPDU{
			createNoSuchObjectPDU(oids[0]),
			createNoSuchObjectPDU(oids[1]),
		})
		profile := &ddsnmp.Profile{
			SourceFile: "bgp-scalar.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{
				scalarBGPTestConfig(),
			}},
		}

		var reports []AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				reports = append(reports, value)
			}),
		})
		for range 2 {
			_, err := collector.Collect()
			require.NoError(t, err)
		}
		require.Len(t, reports, 2)
		for _, report := range reports {
			require.Len(t, report.Routes, 1)
			assert.Equal(t, AcquisitionRouteOutcomeMissing, report.Routes[0].Outcome)
			assert.Zero(t, report.Routes[0].Rejected)
		}
		assert.Equal(t, AcquisitionRouteSourceGET, reports[0].Routes[0].Source)
		assert.Equal(t, AcquisitionRouteSourceCache, reports[1].Routes[0].Source)
	})
}

func TestCollector_AcquisitionObserverReportsCachedAndRejectedTables(t *testing.T) {
	const (
		tableOID  = "1.3.6.1.4.1.99999.3"
		columnOID = tableOID + ".1.1"
		rowOID    = columnOID + ".7"
	)

	t.Run("cached", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()
		profile := createTestProfile("cached.yaml", []ddprofiledefinition.MetricsConfig{{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "cacheTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: columnOID, Name: "value"}},
		}})
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{createGauge32PDU(rowOID, 1)})
		expectSNMPGet(mockHandler, []string{rowOID}, []gosnmp.SnmpPDU{createGauge32PDU(rowOID, 2)})

		var reports []AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				reports = append(reports, report)
			}),
		})
		for range 2 {
			_, err := collector.Collect()
			require.NoError(t, err)
		}

		require.Len(t, reports, 2)
		assert.Equal(t, AcquisitionRouteSourceWalk, reports[0].Routes[0].Source)
		assert.Equal(t, AcquisitionRouteOutcomeValues, reports[0].Routes[0].Outcome)
		assert.Equal(t, AcquisitionRouteSourceCache, reports[1].Routes[0].Source)
		assert.Equal(t, AcquisitionRouteOutcomeValues, reports[1].Routes[0].Outcome)
		assert.Equal(t, uint64(1), reports[1].Routes[0].Rows)
	})

	t.Run("rejected", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()
		profile := createTestProfile("rejected.yaml", []ddprofiledefinition.MetricsConfig{{
			Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "rejectedTable"},
			Symbols: []ddprofiledefinition.SymbolConfig{{OID: columnOID, Name: "value"}},
		}})
		expectSNMPWalk(mockHandler, gosnmp.Version2c, tableOID, []gosnmp.SnmpPDU{createStringPDU(rowOID, "not-a-number")})

		var report AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				report = value
			}),
		})
		_, err := collector.Collect()
		require.NoError(t, err)

		require.Len(t, report.Routes, 1)
		assert.Equal(t, AcquisitionProfileOutcomePartial, report.Outcome)
		assert.Equal(t, AcquisitionRouteOutcomeRejected, report.Routes[0].Outcome)
		assert.Equal(t, uint64(1), report.Routes[0].Rows)
		assert.Equal(t, uint64(1), report.Routes[0].Rejected)
	})
}

func TestCollector_AcquisitionObserverReportsPartialBGPCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler,
		[]string{"1.3.6.1.4.1.99999.20.1.0", "1.3.6.1.4.1.99999.20.2.0"},
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.99999.20.1.0", 6),
			createGauge32PDU("1.3.6.1.4.1.99999.20.2.0", 3600),
		},
	)
	expectSNMPWalkError(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.30.1", errors.New("private BGP timeout"))

	profile := &ddsnmp.Profile{
		SourceFile: "bgp.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{
			scalarBGPTestConfig(),
			tableBGPTestConfig(),
		}},
	}
	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].BGPRows, 1)
	require.Len(t, report.Routes, 2)
	require.Equal(t, []AcquisitionValueReference{{RouteOrdinal: 0}}, report.BGPValueReferences)
	assert.Equal(t, AcquisitionProfileOutcomePartial, report.Outcome)
	assert.Equal(t, AcquisitionRouteKindBGPScalar, report.Routes[0].Kind)
	assert.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionRouteKindBGPTable, report.Routes[1].Kind)
	assert.Equal(t, AcquisitionRouteOutcomeFailed, report.Routes[1].Outcome)
	assert.Equal(t, AcquisitionFailureClassTransport, report.Routes[1].FailureClass)
	assert.NotContains(t, report.String(), "private BGP timeout")
}

func TestCollector_AcquisitionObserverReportsMixedBGPScalarMissingInputs(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		stateOID  = "1.3.6.1.4.1.99999.20.1.0"
		uptimeOID = "1.3.6.1.4.1.99999.20.2.0"
	)
	expectSNMPGet(mockHandler, []string{stateOID, uptimeOID}, []gosnmp.SnmpPDU{
		createNoSuchObjectPDU(stateOID),
		createGauge32PDU(uptimeOID, 3600),
	})
	expectSNMPGet(mockHandler, []string{uptimeOID}, []gosnmp.SnmpPDU{
		createGauge32PDU(uptimeOID, 3600),
	})

	profile := &ddsnmp.Profile{
		SourceFile: "bgp-mixed-missing.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{
			scalarBGPTestConfig(),
		}},
	}
	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, value)
		}),
	})
	for range 2 {
		metrics, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, metrics, 1)
		require.Len(t, metrics[0].BGPRows, 1)
	}

	require.Len(t, reports, 2)
	for _, report := range reports {
		require.Len(t, report.Routes, 1)
		assert.Equal(t, AcquisitionRouteSourceGET, report.Routes[0].Source)
		assert.Equal(t, AcquisitionRouteOutcomePartial, report.Routes[0].Outcome)
		assert.Equal(t, AcquisitionFailureClassDependency, report.Routes[0].FailureClass)
		assert.Equal(t, uint64(1), report.Routes[0].Missing)
		assert.Equal(t, uint64(1), report.Routes[0].Values)
	}
}

func TestAcquisitionProfileRouteDigestIncludesAllBGPValueSources(t *testing.T) {
	tests := map[string]func(*ddprofiledefinition.BGPConfig){
		"identity": func(cfg *ddprofiledefinition.BGPConfig) {
			cfg.Identity.RemoteAS.Symbol.OID = "1.3.6.1.4.1.99999.90.1"
		},
		"descriptor": func(cfg *ddprofiledefinition.BGPConfig) {
			cfg.Descriptors.Description.Symbol.OID = "1.3.6.1.4.1.99999.90.2"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			base := scalarBGPTestConfig()
			changed := base.Clone()
			mutate(&changed)
			baseProfile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{
				BGP: []ddprofiledefinition.BGPConfig{base},
			}}
			changedProfile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{
				BGP: []ddprofiledefinition.BGPConfig{changed},
			}}
			assert.NotEqual(t, acquisitionProfileRouteDigest(baseProfile), acquisitionProfileRouteDigest(changedProfile))
		})
	}

	base := tableBGPTestConfig()
	changed := base.Clone()
	changed.Table.Name += "Changed"
	baseProfile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		BGP: []ddprofiledefinition.BGPConfig{base},
	}}
	changedProfile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		BGP: []ddprofiledefinition.BGPConfig{changed},
	}}
	assert.NotEqual(t, acquisitionProfileRouteDigest(baseProfile), acquisitionProfileRouteDigest(changedProfile))
}

func TestCollector_AcquisitionObserverClassifiesBGPTableLookupFailureAsDependency(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := crossTableBGPTestConfig()
	cfg.Identity.RemoteAS.IndexTransform = []ddprofiledefinition.MetricIndexTransform{{Start: 99}}
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.1", []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.99999.70.1.1.4.192.0.2.1.1.1", 42),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.2", []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.99999.70.2.1.4.192.0.2.1", 65001),
		createStringPDU("1.3.6.1.4.1.99999.70.2.2.4.192.0.2.1", "blue"),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "bgp-dependency-processing.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				BGP: []ddprofiledefinition.BGPConfig{cfg},
			},
		}},
		Log: logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Error(t, metrics[0].BGPCollectError)
	require.Len(t, report.Routes, 1)
	assert.Equal(t, AcquisitionRouteOutcomeRejected, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionFailureClassDependency, report.Routes[0].FailureClass)
}

func TestCollector_AcquisitionObserverClassifiesMissingRequiredBGPTableCellAsDependency(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := crossTableBGPTestConfig()
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.1", []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.99999.70.1.1.4.192.0.2.1.1.1", 42),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.2", []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.4.1.99999.70.2.2.4.192.0.2.1", "blue"),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "bgp-required-dependency-missing.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				BGP: []ddprofiledefinition.BGPConfig{cfg},
			},
		}},
		Log: logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Empty(t, metrics[0].BGPRows)
	assert.NoError(t, metrics[0].BGPCollectError)
	require.Len(t, report.Routes, 1)
	assert.Equal(t, AcquisitionRouteOutcomeRejected, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionFailureClassDependency, report.Routes[0].FailureClass)
}

func TestCollector_AcquisitionObserverIgnoresMissingOptionalBGPTableDescriptor(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := crossTableBGPTestConfig()
	cfg.Descriptors.Description = ddprofiledefinition.BGPValueConfig{
		Table:          "vendorBgpPeerTable",
		IndexTransform: cfg.Identity.RemoteAS.IndexTransform,
		Symbol: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.4.1.99999.70.2.3",
			Name: "vendorBgpPeerDescription",
		},
	}
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.1", []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.99999.70.1.1.4.192.0.2.1.1.1", 42),
	})
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.70.2", []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.99999.70.2.1.4.192.0.2.1", 65001),
		createStringPDU("1.3.6.1.4.1.99999.70.2.2.4.192.0.2.1", "blue"),
	})

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "bgp-optional-dependency-missing.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				BGP: []ddprofiledefinition.BGPConfig{cfg},
			},
		}},
		Log: logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].BGPRows, 1)
	assert.Empty(t, metrics[0].BGPRows[0].Descriptors.Description)
	require.Len(t, report.Routes, 1)
	assert.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionFailureClassNone, report.Routes[0].FailureClass)
}

func TestCollector_AcquisitionObserverAssociatesBGPTableValuesWithRoutes(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler,
		[]string{"1.3.6.1.4.1.99999.20.1.0", "1.3.6.1.4.1.99999.20.2.0"},
		[]gosnmp.SnmpPDU{
			createIntegerPDU("1.3.6.1.4.1.99999.20.1.0", 6),
			createGauge32PDU("1.3.6.1.4.1.99999.20.2.0", 3600),
		},
	)
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.99999.30.1", []gosnmp.SnmpPDU{
		createIntegerPDU("1.3.6.1.4.1.99999.30.1.2.42", 6),
		createGauge32PDU("1.3.6.1.4.1.99999.30.1.3.42", 65001),
		createGauge32PDU("1.3.6.1.4.1.99999.30.1.4.42", 7200),
	})

	tableCfg := tableBGPTestConfig()
	tableCfg.Descriptors.Description.Symbol = ddprofiledefinition.SymbolConfig{
		OID:  "1.3.6.1.4.1.99999.30.1.5",
		Name: "optionalDescription",
	}
	profile := &ddsnmp.Profile{
		SourceFile: "bgp-value-references.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{
			scalarBGPTestConfig(),
			tableCfg,
		}},
	}
	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})

	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].BGPRows, 2)
	require.Equal(t, []AcquisitionValueReference{
		{RouteOrdinal: 0},
		{RouteOrdinal: 1, RowOrdinal: 0},
	}, report.BGPValueReferences)
	assert.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[1].Outcome)
	assert.Zero(t, report.Routes[1].Missing, "optional absent BGP descriptors are not missing logical inputs")
}

func acquisitionRoutesByKind(routes []AcquisitionRouteReport) map[AcquisitionRouteKind]AcquisitionRouteReport {
	result := make(map[AcquisitionRouteKind]AcquisitionRouteReport, len(routes))
	for _, route := range routes {
		result[route.Kind] = route
	}
	return result
}
