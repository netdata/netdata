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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, metrics *ddsnmp.ProfileMetrics) {
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

func TestCollector_InitialAcquisitionObserverReportsProfileInputsOnce(t *testing.T) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			reports = append(reports, report)
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "lab", metrics[0].Tags["site"])
	assert.NotContains(t, metrics[0].DeviceMetadata, "serial_number")
	require.Len(t, reports, 1)

	first := acquisitionRoutesByKind(reports[0].Routes)
	require.Contains(t, first, AcquisitionRouteKindProfileTagScalar)
	require.Contains(t, first, AcquisitionRouteKindMetadataScalar)
	assert.Equal(t, AcquisitionRouteSourceGET, first[AcquisitionRouteKindProfileTagScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeValues, first[AcquisitionRouteKindProfileTagScalar].Outcome)
	assert.Equal(t, AcquisitionRouteSourceGET, first[AcquisitionRouteKindMetadataScalar].Source)
	assert.Equal(t, AcquisitionRouteOutcomeMissing, first[AcquisitionRouteKindMetadataScalar].Outcome)

	metrics, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "lab", metrics[0].Tags["site"], "normal collection must continue to reuse its live cache")
	assert.NotContains(t, metrics[0].DeviceMetadata, "serial_number")
	assert.Len(t, reports, 1, "acquisition evidence is produced only by the initial Collect call")
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "lab", metrics[0].Tags["site"])
	assert.Equal(t, "core", metrics[0].Tags["role"])
	require.Len(t, report.Routes, 1)
	assert.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionRouteSourceGET, report.Routes[0].Source)
}

func TestCollector_AcquisitionObserverPanicDoesNotChangeCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{createTestProfile("profile.yaml", nil)},
		Log:        logger.New(),
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {
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
			InitialAcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

func TestAcquisitionProfileRouteDigestIgnoresOrdinaryMetrics(t *testing.T) {
	profile := &ddsnmp.Profile{Definition: &ddprofiledefinition.ProfileDefinition{
		Topology: []ddprofiledefinition.TopologyConfig{{
			Kind: ddsnmp.KindArpEntry,
			MetricsConfig: ddprofiledefinition.MetricsConfig{
				Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3", Name: "topologyValue"},
			},
		}},
	}}

	want := acquisitionProfileRouteDigest(profile)
	profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{{
		Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.4", Name: "ordinaryValue"},
	}}

	assert.Equal(t, want, acquisitionProfileRouteDigest(profile))
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

func TestCollector_AcquisitionObserverLeavesTopologyRouteUnobservedWhenRegularTableFails(t *testing.T) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	_, err := collector.Collect()
	require.Error(t, err)

	assert.Equal(t, AcquisitionProfileOutcomeFailed, report.Outcome)
	assert.Equal(t, AcquisitionFailurePhaseTables, report.FailurePhase)
	require.Len(t, report.Routes, 1)
	assert.Equal(t, topologyTableOID, report.Routes[0].RootOID)
	assert.Equal(t, AcquisitionRouteSourceWalk, report.Routes[0].Source)
	assert.Equal(t, AcquisitionRouteOutcomeNotObserved, report.Routes[0].Outcome)
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
	first := acquisitionTopologyTestProfile("a-current.yaml", ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "currentMissing"},
	})
	second := acquisitionTopologyTestProfile("b-inherited.yaml", ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "inheritedMissing"},
	})
	expectSNMPGet(mockHandler, []string{oid}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(oid)})

	var reports []AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{second, first},
		Log:        logger.New(),
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
	t.Run("topology scalar", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		const oid = "1.3.6.1.4.1.99999.40.1.0"
		profile := acquisitionTopologyTestProfile("topology-scalar.yaml", ddprofiledefinition.MetricsConfig{
			Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "missingMetric"},
		})
		expectSNMPGet(mockHandler, []string{oid}, []gosnmp.SnmpPDU{createNoSuchObjectPDU(oid)})

		var report AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

		var report AcquisitionProfileReport
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
			InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
				report = value
			}),
		})
		_, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, report.Routes, 1)
		assert.Equal(t, AcquisitionRouteOutcomeMissing, report.Routes[0].Outcome)
		assert.Zero(t, report.Routes[0].Rejected)
		assert.Equal(t, AcquisitionRouteSourceGET, report.Routes[0].Source)
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

	profile := &ddsnmp.Profile{
		SourceFile: "bgp-mixed-missing.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{
			scalarBGPTestConfig(),
		}},
	}
	var report AcquisitionProfileReport
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
			report = value
		}),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Len(t, metrics[0].BGPRows, 1)

	require.Len(t, report.Routes, 1)
	assert.Equal(t, AcquisitionRouteSourceGET, report.Routes[0].Source)
	assert.Equal(t, AcquisitionRouteOutcomePartial, report.Routes[0].Outcome)
	assert.Equal(t, AcquisitionFailureClassDependency, report.Routes[0].FailureClass)
	assert.Equal(t, uint64(1), report.Routes[0].Missing)
	assert.Equal(t, uint64(1), report.Routes[0].Values)
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

func TestFirstBGPRouteOIDDoesNotBulkAllocateBySourceCount(t *testing.T) {
	const smallestOID = "1.3.6.1.2.1.1"
	cfg := scalarBGPTestConfig()
	cfg.MetricTags = make(ddprofiledefinition.MetricTagConfigList, 10_000)
	for i := range cfg.MetricTags {
		cfg.MetricTags[i].Symbol = ddprofiledefinition.SymbolConfigCompat{
			OID:  "1.3.6.1.4.1.99999.100",
			Name: "tagSource",
		}
	}
	cfg.MetricTags[0].Symbol.OID = smallestOID

	allocationCount := func(cfg ddprofiledefinition.BGPConfig) float64 {
		var got string
		allocations := testing.AllocsPerRun(10, func() {
			got = firstBGPRouteOID(cfg)
		})
		require.Equal(t, smallestOID, got)
		return allocations
	}
	small := cfg
	small.MetricTags = cfg.MetricTags[:1]

	assert.LessOrEqual(t, allocationCount(cfg), allocationCount(small)+1,
		"representative-root selection must not copy configured source OIDs")
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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
		InitialAcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
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

func acquisitionTopologyTestProfile(source string, config ddprofiledefinition.MetricsConfig) *ddsnmp.Profile {
	return &ddsnmp.Profile{
		SourceFile: source,
		Definition: &ddprofiledefinition.ProfileDefinition{Topology: []ddprofiledefinition.TopologyConfig{{
			Kind:          ddsnmp.KindArpEntry,
			MetricsConfig: config,
		}}},
	}
}
