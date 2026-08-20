// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	snmpmock "github.com/gosnmp/gosnmp/mocks"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// statefulSNMPDevice models device state across collection cycles without
// coupling the tests to a specific cache wire strategy.
type statefulSNMPDevice struct {
	values    map[string]gosnmp.SnmpPDU
	getErrs   map[string]error
	walkErrs  map[string]error
	walkCount map[string]int
}

func newStatefulSNMPDevice() *statefulSNMPDevice {
	return &statefulSNMPDevice{
		values:    make(map[string]gosnmp.SnmpPDU),
		getErrs:   make(map[string]error),
		walkErrs:  make(map[string]error),
		walkCount: make(map[string]int),
	}
}

func (d *statefulSNMPDevice) install(mockHandler *snmpmock.MockHandler) {
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	mockHandler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
		root := trimOID(oid)
		d.walkCount[root]++
		if err := d.walkErrs[root]; err != nil {
			return nil, err
		}
		var pdus []gosnmp.SnmpPDU
		for name, pdu := range d.values {
			if strings.HasPrefix(name, root+".") {
				pdus = append(pdus, pdu)
			}
		}
		sort.Slice(pdus, func(i, j int) bool { return pdus[i].Name < pdus[j].Name })
		return pdus, nil
	}).AnyTimes()
	mockHandler.EXPECT().Get(gomock.Any()).DoAndReturn(func(oids []string) (*gosnmp.SnmpPacket, error) {
		for _, oid := range oids {
			if err := d.getErrs[trimOID(oid)]; err != nil {
				return nil, err
			}
		}
		vars := make([]gosnmp.SnmpPDU, 0, len(oids))
		for _, oid := range oids {
			if pdu, ok := d.values[trimOID(oid)]; ok {
				vars = append(vars, pdu)
			} else {
				vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.NoSuchInstance})
			}
		}
		return &gosnmp.SnmpPacket{Variables: vars}, nil
	}).AnyTimes()
}

func (d *statefulSNMPDevice) set(pdu gosnmp.SnmpPDU) {
	d.values[trimOID(pdu.Name)] = pdu
}

func TestCollector_Collect_AlternatingOmissionDiscardsStaleDependent(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	const (
		ownerScalarOID  = "1.3.6.1.4.1.99999.101.0"
		sourceScalarOID = "1.3.6.1.4.1.99999.102.0"
		sourceTableOID  = "1.3.6.1.4.1.99999.1"
		sourceColumnOID = sourceTableOID + ".1"
		sourceMetricOID = sourceColumnOID + ".1"
		sharedTableOID  = "1.3.6.1.4.1.99999.2"
		ownerColumnOID  = sharedTableOID + ".1.1"
		ownerMetricOID  = ownerColumnOID + ".1"
		depColumnOID    = sharedTableOID + ".1.5"
		depMetricOID    = depColumnOID + ".1"
	)

	ownerConfig := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: sharedTableOID, Name: "sharedTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: ownerColumnOID, Name: "ownerValue"}},
	}
	sourceConfig := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: sourceTableOID, Name: "sourceTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: sourceColumnOID, Name: "sourceValue"}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag:   "dep_name",
			Table: "sharedTable",
			Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID: depColumnOID, Name: "depName",
			},
		}},
	}
	ownerProfile := createTestProfile("a-owner.yaml", []ddprofiledefinition.MetricsConfig{
		createScalarMetric(ownerScalarOID, "ownerScalar"), ownerConfig,
	})
	sourceProfile := createTestProfile("b-source.yaml", []ddprofiledefinition.MetricsConfig{
		createScalarMetric(sourceScalarOID, "sourceScalar"), sourceConfig,
	})
	ddsnmp.HandleCrossTableTagsWithoutMetrics(sourceProfile)

	device := newStatefulSNMPDevice()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles:   []*ddsnmp.Profile{ownerProfile, sourceProfile},
		Log:        logger.New(),
	})

	findMetric := func(results []*ddsnmp.ProfileMetrics, source, name string) *ddsnmp.Metric {
		for _, result := range results {
			if result.Source != source {
				continue
			}
			for i := range result.Metrics {
				if result.Metrics[i].Name == name {
					return &result.Metrics[i]
				}
			}
		}
		return nil
	}

	device.getErrs[ownerScalarOID] = errors.New("owner scalar timeout")
	device.set(createGauge32PDU(sourceScalarOID, 1))
	device.set(createGauge32PDU(sourceMetricOID, 10))
	device.set(createGauge32PDU(ownerMetricOID, 100))
	device.set(createStringPDU(depMetricOID, "alpha"))

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	metric := findMetric(results, sourceProfile.SourceFile, "sourceValue")
	require.NotNil(t, metric)
	assert.Equal(t, "alpha", metric.Tags["dep_name"])
	require.True(t, collector.tableCache.isConfigCached(sourceConfig))

	delete(device.getErrs, ownerScalarOID)
	device.getErrs[sourceScalarOID] = errors.New("source scalar timeout")
	device.set(createGauge32PDU(ownerScalarOID, 1))
	device.set(createStringPDU(depMetricOID, "beta"))

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, ownerProfile.SourceFile, results[0].Source)
	require.False(t, collector.tableCache.isConfigCached(sourceConfig))

	delete(device.getErrs, sourceScalarOID)
	device.set(createGauge32PDU(sourceMetricOID, 12))
	device.set(createGauge32PDU(ownerMetricOID, 110))
	device.set(createStringPDU(depMetricOID, "gamma"))

	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 2)
	metric = findMetric(results, sourceProfile.SourceFile, "sourceValue")
	require.NotNil(t, metric)
	assert.Equal(t, "gamma", metric.Tags["dep_name"])
	ownerMetric := findMetric(results, ownerProfile.SourceFile, "ownerValue")
	require.NotNil(t, ownerMetric)
	assert.EqualValues(t, 110, ownerMetric.Value)
}

func TestCollector_Collect_BGPIneligibleWalkIsNotCached(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := tableBGPTestConfig()
	device := newStatefulSNMPDevice()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "vendor-device.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{cfg}},
		}},
		Log: logger.New(),
	})

	device.set(createGauge32PDU("1.3.6.1.4.1.99999.30.1.9.42", 5))
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Empty(t, results[0].BGPRows)
	require.False(t, collector.tableCache.isConfigCached(bgpConfigAsMetricsConfig(cfg)))

	device.set(createIntegerPDU("1.3.6.1.4.1.99999.30.1.2.42", 6))
	device.set(createGauge32PDU("1.3.6.1.4.1.99999.30.1.3.42", 65001))
	device.set(createGauge32PDU("1.3.6.1.4.1.99999.30.1.4.42", 7200))
	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].BGPRows, 1)
	assert.Equal(t, "42", results[0].BGPRows[0].Identity.Neighbor)
	assert.Equal(t, 2, device.walkCount["1.3.6.1.4.1.99999.30.1"])
}

func TestCollector_Collect_LicensingIneligibleWalkIsNotCached(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := ddprofiledefinition.LicensingConfig{
		OriginProfileID: "_vendor-licensing.yaml",
		Table:           ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3", Name: "vendorLicenseTable"},
		Identity: ddprofiledefinition.LicenseIdentityConfig{
			ID: ddprofiledefinition.LicenseValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3.1", Name: "licenseID"},
			},
		},
		State: ddprofiledefinition.LicenseStateConfig{
			LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3.3", Name: "licenseState"},
			},
			Policy: ddprofiledefinition.LicenseStatePolicyDefault,
		},
	}
	device := newStatefulSNMPDevice()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "vendor-device.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{Licensing: []ddprofiledefinition.LicensingConfig{cfg}},
		}},
		Log: logger.New(),
	})

	device.set(createStringPDU("1.3.6.1.4.1.99999.3.9.7", "noise"))
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Empty(t, results[0].LicenseRows)
	require.False(t, collector.tableCache.isConfigCached(licensingConfigAsMetricsConfig(cfg)))

	device.set(createStringPDU("1.3.6.1.4.1.99999.3.1.7", "security"))
	device.set(createIntegerPDU("1.3.6.1.4.1.99999.3.3.7", 1))
	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].LicenseRows, 1)
	assert.Equal(t, "security", results[0].LicenseRows[0].ID)
	assert.Equal(t, 2, device.walkCount["1.3.6.1.4.1.99999.3"])
}

func TestCollector_Collect_SharesEmptyDirectTableWalkPerPass(t *testing.T) {
	t.Run("BGP", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		first := tableBGPTestConfig()
		second := first
		second.ID = "second-peer-view"
		device := newStatefulSNMPDevice()
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					BGP: []ddprofiledefinition.BGPConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].BGPRows)
		assert.Equal(t, 1, device.walkCount[trimOID(first.Table.OID)])
		assert.EqualValues(t, 1, results[0].Stats.SNMP.TablesWalked)
	})

	t.Run("licensing", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		first := ddprofiledefinition.LicensingConfig{
			ID:    "first-license-view",
			Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3", Name: "vendorLicenseTable"},
			Identity: ddprofiledefinition.LicenseIdentityConfig{
				ID: ddprofiledefinition.LicenseValueConfig{
					Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3.1", Name: "licenseID"},
				},
			},
		}
		second := first
		second.ID = "second-license-view"
		device := newStatefulSNMPDevice()
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Licensing: []ddprofiledefinition.LicensingConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].LicenseRows)
		assert.Equal(t, 1, device.walkCount[trimOID(first.Table.OID)])
		assert.EqualValues(t, 1, results[0].Stats.SNMP.TablesWalked)
	})
}

func TestCollector_Collect_SharesFailedDirectTableWalkPerPass(t *testing.T) {
	t.Run("BGP primary", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		first := tableBGPTestConfig()
		second := first
		second.ID = "second-peer-view"
		device := newStatefulSNMPDevice()
		device.walkErrs[trimOID(first.Table.OID)] = errors.New("BGP table timeout")
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					BGP: []ddprofiledefinition.BGPConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].BGPRows)
		require.Error(t, results[0].BGPCollectError)
		assert.Equal(t, 1, device.walkCount[trimOID(first.Table.OID)])
		assert.EqualValues(t, 1, results[0].Stats.SNMP.WalkRequests)
		assert.EqualValues(t, 1, results[0].Stats.Errors.SNMP)
		assert.Zero(t, results[0].Stats.SNMP.TablesWalked)
	})

	t.Run("BGP dependency", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		first := crossTableBGPTestConfig()
		second := first
		second.ID = "second-peer-family-view"
		const (
			primaryOID    = "1.3.6.1.4.1.99999.70.1"
			dependencyOID = "1.3.6.1.4.1.99999.70.2"
		)
		device := newStatefulSNMPDevice()
		device.set(createGauge32PDU(primaryOID+".1.4.192.0.2.1.1.1", 42))
		device.walkErrs[dependencyOID] = errors.New("BGP dependency timeout")
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					BGP: []ddprofiledefinition.BGPConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].BGPRows)
		require.Error(t, results[0].BGPCollectError)
		assert.Equal(t, 1, device.walkCount[primaryOID])
		assert.Equal(t, 1, device.walkCount[dependencyOID])
		assert.EqualValues(t, 2, results[0].Stats.SNMP.WalkRequests)
		assert.EqualValues(t, 1, results[0].Stats.Errors.SNMP)
		assert.EqualValues(t, 1, results[0].Stats.SNMP.TablesWalked)
	})

	t.Run("licensing primary", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		first := ddprofiledefinition.LicensingConfig{
			ID:    "first-license-view",
			Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3", Name: "vendorLicenseTable"},
			Identity: ddprofiledefinition.LicenseIdentityConfig{
				ID: ddprofiledefinition.LicenseValueConfig{
					Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.3.1", Name: "licenseID"},
				},
			},
		}
		second := first
		second.ID = "second-license-view"
		device := newStatefulSNMPDevice()
		device.walkErrs[trimOID(first.Table.OID)] = errors.New("licensing table timeout")
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Licensing: []ddprofiledefinition.LicensingConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].LicenseRows)
		assert.Equal(t, 1, device.walkCount[trimOID(first.Table.OID)])
		assert.EqualValues(t, 1, results[0].Stats.SNMP.WalkRequests)
		assert.EqualValues(t, 1, results[0].Stats.Errors.SNMP)
		assert.Zero(t, results[0].Stats.SNMP.TablesWalked)
	})

	t.Run("licensing dependency", func(t *testing.T) {
		ctrl, mockHandler := setupMockHandler(t)
		defer ctrl.Finish()

		const (
			primaryOID    = "1.3.6.1.4.1.99999.6"
			dependencyOID = "1.3.6.1.2.1.31.1.1.1.1"
		)
		first := ddprofiledefinition.LicensingConfig{
			ID:    "first-license-view",
			Table: ddprofiledefinition.SymbolConfig{OID: primaryOID, Name: "licenseIfTable"},
			Identity: ddprofiledefinition.LicenseIdentityConfig{
				ID: ddprofiledefinition.LicenseValueConfig{Index: 1},
			},
			State: ddprofiledefinition.LicenseStateConfig{
				LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{
					Symbol: ddprofiledefinition.SymbolConfig{OID: primaryOID + ".1", Name: "licenseState"},
				},
				Policy: ddprofiledefinition.LicenseStatePolicyDefault,
			},
			MetricTags: ddprofiledefinition.MetricTagConfigList{{
				Tag:   "if_name",
				Table: "ifXTable",
				Symbol: ddprofiledefinition.SymbolConfigCompat{
					OID: dependencyOID, Name: "ifName",
				},
			}},
		}
		second := first
		second.ID = "second-license-view"
		device := newStatefulSNMPDevice()
		device.set(createIntegerPDU(primaryOID+".1.2", 1))
		device.walkErrs[dependencyOID] = errors.New("licensing dependency timeout")
		device.install(mockHandler)
		collector := New(Config{
			SnmpClient: mockHandler,
			Profiles: []*ddsnmp.Profile{{
				SourceFile: "vendor-device.yaml",
				Definition: &ddprofiledefinition.ProfileDefinition{
					Licensing: []ddprofiledefinition.LicensingConfig{first, second},
				},
			}},
			Log: logger.New(),
		})

		results, err := collector.Collect()
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, results[0].LicenseRows)
		assert.Equal(t, 1, device.walkCount[primaryOID])
		assert.Equal(t, 1, device.walkCount[dependencyOID])
		assert.EqualValues(t, 2, results[0].Stats.SNMP.WalkRequests)
		assert.EqualValues(t, 1, results[0].Stats.Errors.SNMP)
		assert.EqualValues(t, 1, results[0].Stats.SNMP.TablesWalked)
	})
}

func TestCollector_Collect_BGPCrossTableTagIsCurrentOnEveryCollection(t *testing.T) {
	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	cfg := ddprofiledefinition.BGPConfig{
		OriginProfileID: "_vendor-bgp.yaml",
		ID:              "xtag-peer",
		Kind:            ddprofiledefinition.BGPRowKindPeer,
		Table:           ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.4.1.99999.80.1", Name: "xtagPeerTable"},
		Identity: ddprofiledefinition.BGPIdentityConfig{
			Neighbor: ddprofiledefinition.BGPValueConfig{Index: 1},
			RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
		},
		State: ddprofiledefinition.BGPStateConfig{
			BGPValueConfig: ddprofiledefinition.BGPValueConfig{
				Symbol: ddprofiledefinition.SymbolConfig{
					OID: "1.3.6.1.4.1.99999.80.1.2", Name: "xtagPeerState", Mapping: bgpPeerStateMapping(),
				},
			},
		},
		MetricTags: []ddprofiledefinition.MetricTagConfig{{
			Tag:   "peer_group",
			Table: "vendorPeerGroupTable",
			Symbol: ddprofiledefinition.SymbolConfigCompat{
				OID: "1.3.6.1.4.1.99999.81.1.1", Name: "vendorPeerGroupName",
			},
		}},
	}
	device := newStatefulSNMPDevice()
	device.install(mockHandler)
	collector := New(Config{
		SnmpClient: mockHandler,
		Profiles: []*ddsnmp.Profile{{
			SourceFile: "vendor-device.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{BGP: []ddprofiledefinition.BGPConfig{cfg}},
		}},
		Log: logger.New(),
	})

	device.set(createIntegerPDU("1.3.6.1.4.1.99999.80.1.2.7", 6))
	device.set(createStringPDU("1.3.6.1.4.1.99999.81.1.1.7", "red"))
	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].BGPRows, 1)
	assert.Equal(t, "red", results[0].BGPRows[0].Tags["peer_group"])

	device.set(createStringPDU("1.3.6.1.4.1.99999.81.1.1.7", "blue"))
	results, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].BGPRows, 1)
	assert.Equal(t, "blue", results[0].BGPRows[0].Tags["peer_group"])
}

func TestTableCollector_OrganizePDUsByRow_AllocatesCacheMapsOnlyWhenEligible(t *testing.T) {
	const (
		tableOID  = "1.3.6.1.4.1.99999.90"
		columnOID = tableOID + ".1"
		rowOID    = columnOID + ".7"
	)
	collector := newTableCollector(nil, make(map[string]bool), newTableCache(time.Hour, 0), logger.New(), false)
	config := ddprofiledefinition.MetricsConfig{
		Table:   ddprofiledefinition.SymbolConfig{OID: tableOID, Name: "allocationTable"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: columnOID, Name: "value"}},
	}

	organize := func(cacheStructure bool) *tableProcessingContext {
		ctx := &tableProcessingContext{
			config:         config,
			pdus:           map[string]gosnmp.SnmpPDU{rowOID: createGauge32PDU(rowOID, 1)},
			columnOIDs:     buildColumnOIDs(config),
			orderedTags:    buildOrderedTags(config),
			cacheStructure: cacheStructure,
		}
		ctx.rows, ctx.oidCache, ctx.tagCache = collector.organizePDUsByRow(ctx)
		return ctx
	}

	ineligible := organize(false)
	require.Len(t, ineligible.rows, 1)
	assert.Nil(t, ineligible.oidCache)
	assert.Nil(t, ineligible.tagCache)

	eligible := organize(true)
	require.Len(t, eligible.rows, 1)
	assert.Equal(t, rowOID, eligible.oidCache["7"][columnOID])
	assert.NotNil(t, eligible.tagCache["7"])
}
