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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestRefreshProfileInputsPreservesPollingState(t *testing.T) {
	const (
		nameOID    = snmputils.OidSysName
		missingOID = "1.3.6.1.4.1.99999.91.0"
		root       = "1.3.6.1.4.1.99999.92"
	)
	for name, initiallyMissing := range map[string]bool{
		"changed observed name":              false,
		"observed name contradicts negative": true,
	} {
		t.Run(name, func(t *testing.T) {
			currentName := "old"
			var requested []string
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					var pdus []gosnmp.SnmpPDU
					for _, oid := range oids {
						requested = append(requested, oid)
						switch oid {
						case nameOID:
							if initiallyMissing && currentName == "old" {
								pdus = append(pdus, createNoSuchObjectPDU(oid))
							} else {
								pdus = append(pdus, createStringPDU(oid, currentName))
							}
						case missingOID:
							pdus = append(pdus, createNoSuchObjectPDU(oid))
						case root + ".1.1":
							pdus = append(pdus, createGauge32PDU(oid, 7))
						default:
							t.Fatalf("unexpected GET OID %q", oid)
						}
					}
					return &gosnmp.SnmpPacket{
						Variables: pdus,
					}, nil
				},
				walk: func(oid string) ([]gosnmp.SnmpPDU, error) {
					require.Equal(t, root, oid)
					return []gosnmp.SnmpPDU{createGauge32PDU(root+".1.1", 7), createStringPDU(root+".2.1", "eth0")}, nil
				},
			}
			table := ddprofiledefinition.MetricsConfig{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  root,
					Name: "table",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}},
				MetricTags: []ddprofiledefinition.MetricTagConfig{{
					Tag: "interface", Symbol: ddprofiledefinition.SymbolConfigCompat{
						OID: root + ".2",
					},
				}},
			}
			profile := refreshInputProfile("device.yaml", nameOID)
			profile.Definition.Metrics = []ddprofiledefinition.MetricsConfig{
				createScalarMetric(missingOID, "missing"),
				table,
			}
			var reports []AcquisitionProfileReport
			first := &SourceRecorder{
				ContextID: 1,
			}
			collector := New(Config{
				SnmpClient: first.Wrap(handler),
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(
					func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) {
						reports = append(reports, report)
					},
				),
			})
			assert.Empty(t, collector.PreparedDeviceMetadata())
			_, err := collector.Collect()
			require.NoError(t, err)
			first.Finish()
			require.Equal(t, 1, handler.walks)
			require.True(t, collector.missingOIDs[missingOID])
			require.Equal(t, initiallyMissing, collector.missingOIDs[nameOID])
			assert.Equal(t, initiallyMissing, collector.NeedsProfileInputRefresh([]string{"." + nameOID}))
			cache := collector.tableCache
			timestamp := cache.timestamps[root]
			oids, tags, found := cache.getCachedData(table)
			require.True(t, found)
			require.Equal(t, "eth0", tags["1"]["interface"])
			unrelatedCause := collector.negative.causes[missingOID]
			oldInputs := collector.CachedInputs()

			currentName = "new"
			requested = nil
			second := &SourceRecorder{
				ContextID: 2,
			}
			collector.SetSNMPClient(second.Wrap(handler))
			changed, refreshErr := collector.RefreshProfileInputs([]string{"." + nameOID})
			require.NoError(t, refreshErr)
			assert.True(t, changed)
			assert.False(t, collector.NeedsProfileInputRefresh([]string{nameOID}))
			require.Equal(t, []string{nameOID, nameOID}, requested)
			second.Finish()
			assert.Len(t, reports, 1, "refresh is not an additional metric collection report")
			assert.Same(t, cache, collector.tableCache)
			assert.Equal(t, timestamp, cache.timestamps[root])
			newOIDs, newTags, found := cache.getCachedData(table)
			require.True(t, found)
			assert.Equal(t, oids, newOIDs)
			assert.Equal(t, tags, newTags)
			assert.Equal(t, 1, handler.walks)
			assert.True(t, collector.missingOIDs[missingOID])
			assert.NotContains(t, collector.missingOIDs, nameOID)
			assert.Equal(t, unrelatedCause, collector.negative.causes[missingOID])
			assert.NotContains(t, collector.negative.causes, nameOID)
			metadata := collector.PreparedDeviceMetadata()
			assert.Equal(t, "new", metadata["name"].Value)
			metadata["name"] = ddsnmp.MetaTag{
				Value: "caller edit",
			}
			assert.Equal(t, "new", collector.PreparedDeviceMetadata()["name"].Value)
			for _, input := range collector.CachedInputs() {
				if input.Kind == "profile_tags" || input.Kind == "profile_metadata" {
					require.Len(t, input.Sources, 1)
					assert.EqualValues(t, 2, input.Sources[0].ContextID)
				}
			}
			for _, input := range oldInputs {
				if len(input.Sources) > 0 {
					assert.EqualValues(t, 1, input.Sources[0].ContextID)
				}
			}

			requested = nil
			third := &SourceRecorder{
				ContextID: 3,
			}
			collector.SetSNMPClient(third.Wrap(handler))
			metrics, err := collector.Collect()
			require.NoError(t, err)
			third.Finish()
			require.Len(t, metrics, 1)
			assert.Equal(t, "new", metrics[0].Tags["snmp_host"])
			assert.Equal(t, "new", metrics[0].DeviceMetadata["name"].Value)
			assert.Equal(
				t,
				[]string{root + ".1.1"},
				requested,
				"normal collection reuses refreshed scalar inputs and table structure",
			)
			require.Len(t, reports, 2)
			for _, route := range reports[1].Routes {
				if route.Kind == AcquisitionRouteKindProfileTagScalar ||
					route.Kind == AcquisitionRouteKindMetadataScalar {
					assert.Equal(t, AcquisitionRouteSourceCache, route.Source)
					require.Len(t, route.Sources, 1)
					assert.EqualValues(t, 2, route.Sources[0].ContextID)
				}
			}
		})
	}
}

func TestRefreshProfileInputsRetainsFailedProfiles(t *testing.T) {
	const badOID, goodOID = "1.2.3.0", "1.2.4.0"
	for name, failure := range map[string]string{
		"tag processing with static tag":        "tag",
		"metadata processing with static field": "metadata",
		"transport":                             "transport",
	} {
		t.Run(name, func(t *testing.T) {
			refresh := false
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					oid := oids[0]
					if refresh && oid == badOID {
						if failure == "transport" {
							return nil, errors.New("input timeout")
						}
						return &gosnmp.SnmpPacket{
							Variables: []gosnmp.SnmpPDU{createIntegerPDU(oid, 42)},
						}, nil
					}
					value := "old"
					if refresh {
						value = "new"
					}
					return &gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{createStringPDU(oid, value)},
					}, nil
				},
			}
			bad := refreshInputProfile("a.yaml", badOID)
			bad.Definition.StaticTags = []ddprofiledefinition.StaticMetricTagConfig{{Tag: "static", Value: "present"}}
			bad.Definition.Metadata["device"].Fields["vendor"] = ddprofiledefinition.MetadataField{
				Value: "present",
			}
			if failure == "tag" {
				bad.Definition.MetricTags[0].Symbol.Format = "hex"
			} else if failure == "metadata" {
				field := bad.Definition.Metadata["device"].Fields["name"]
				field.Symbol.Format = "hex"
				bad.Definition.Metadata["device"].Fields["name"] = field
			}
			good := refreshInputProfile("b.yaml", goodOID)
			first := &SourceRecorder{
				ContextID: 1,
			}
			collector := New(Config{
				SnmpClient:          first.Wrap(handler),
				Profiles:            []*ddsnmp.Profile{bad, good},
				Log:                 logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(func(AcquisitionProfileReport, *ddsnmp.ProfileMetrics) {}),
			})
			_, err := collector.Collect()
			require.NoError(t, err)
			first.Finish()
			previous := collector.profiles[bad.SourceFile].cache
			refresh = true
			second := &SourceRecorder{
				ContextID: 2,
			}
			collector.SetSNMPClient(second.Wrap(handler))
			changed, refreshErr := collector.RefreshProfileInputs(nil)
			require.ErrorContains(t, refreshErr, "a.yaml")
			assert.True(t, changed, "successful profile snapshots change even when another profile fails")
			second.Finish()
			assert.Equal(t, previous, collector.profiles[bad.SourceFile].cache)
			assert.Equal(t, "new", collector.profiles[good.SourceFile].cache.deviceMetadata["name"].Value)
			assert.EqualValues(t, 1, collector.CollectionFailures().Profiles.Count)
			for _, input := range collector.CachedInputs() {
				require.Len(t, input.Sources, 1)
				if input.Profile == bad.SourceFile {
					assert.EqualValues(t, 1, input.Sources[0].ContextID)
				} else {
					assert.EqualValues(t, 2, input.Sources[0].ContextID)
				}
			}
			third := &SourceRecorder{
				ContextID: 3,
			}
			collector.SetSNMPClient(third.Wrap(handler))
			metrics, err := collector.Collect()
			require.NoError(t, err, "failed input refresh does not block ordinary healthy profiles")
			third.Finish()
			require.Len(t, metrics, 2)
			assert.Equal(t, "new", metrics[1].Tags["snmp_host"])
			assert.Equal(t, "new", metrics[1].DeviceMetadata["name"].Value)
		})
	}
}

func TestRefreshProfileInputsDoesNotTightenStartup(t *testing.T) {
	const oid = "1.2.3.0"
	for name, metadataFailure := range map[string]bool{"tag failure": false, "metadata failure": true} {
		t.Run(name, func(t *testing.T) {
			profile := refreshInputProfile("device.yaml", oid)
			profile.Definition.StaticTags = []ddprofiledefinition.StaticMetricTagConfig{
				{Tag: "static", Value: "present"},
			}
			profile.Definition.Metadata["device"].Fields["vendor"] = ddprofiledefinition.MetadataField{
				Value: "fixture",
			}
			if metadataFailure {
				field := profile.Definition.Metadata["device"].Fields["name"]
				field.Symbol.Format = "hex"
				profile.Definition.Metadata["device"].Fields["name"] = field
			} else {
				profile.Definition.MetricTags[0].Symbol.Format = "hex"
			}
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					return &gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{createIntegerPDU(oids[0], 42)},
					}, nil
				},
			}
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			metrics, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, "present", metrics[0].Tags["static"])
			assert.Equal(t, "fixture", metrics[0].DeviceMetadata["vendor"].Value)
			_, refreshErr := collector.RefreshProfileInputs(nil)
			require.Error(t, refreshErr)
			metrics, err = collector.Collect()
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Equal(t, "fixture", metrics[0].DeviceMetadata["vendor"].Value)
		})
	}
}

func TestRefreshProfileInputsRetainsUnansweredSnapshots(t *testing.T) {
	const oid = "1.2.3.0"
	for name, omittedRequest := range map[string]int{"global tag response": 1, "metadata response": 2} {
		t.Run(name, func(t *testing.T) {
			value := "old"
			requests := 0
			omit := 0
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					requests++
					if requests == omit {
						return &gosnmp.SnmpPacket{}, nil
					}
					return &gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{createStringPDU(oids[0], value)},
					}, nil
				},
			}
			profile := refreshInputProfile("device.yaml", oid)
			profile.Definition.StaticTags = []ddprofiledefinition.StaticMetricTagConfig{
				{Tag: "static", Value: "present"},
			}
			profile.Definition.Metadata["device"].Fields["vendor"] = ddprofiledefinition.MetadataField{
				Value: "fixture",
			}
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			_, err := collector.Collect()
			require.NoError(t, err)
			value, requests, omit = "new", 0, omittedRequest
			changed, refreshErr := collector.RefreshProfileInputs(nil)
			require.ErrorContains(t, refreshErr, "omitted requested profile input")
			assert.False(t, changed)
			assert.Equal(t, "old", collector.PreparedDeviceMetadata()["name"].Value)
			assert.Equal(t, "old", collector.profiles[profile.SourceFile].cache.globalTags["snmp_host"])
			assert.NotContains(t, collector.missingOIDs, oid)
			omit = 0
			changed, refreshErr = collector.RefreshProfileInputs(nil)
			require.NoError(t, refreshErr)
			assert.True(t, changed)
			assert.Equal(t, "new", collector.PreparedDeviceMetadata()["name"].Value)
			assert.Equal(t, "new", collector.profiles[profile.SourceFile].cache.globalTags["snmp_host"])
		})
	}
}

func TestRefreshProfileInputsKeepsUnansweredStartupPolicy(t *testing.T) {
	profile := refreshInputProfile("device.yaml", "1.2.3.0")
	profile.Definition.StaticTags = []ddprofiledefinition.StaticMetricTagConfig{{Tag: "static", Value: "present"}}
	profile.Definition.Metadata["device"].Fields["vendor"] = ddprofiledefinition.MetadataField{
		Value: "fixture",
	}
	handler := &sourceTestHandler{
		get: func([]string) (*gosnmp.SnmpPacket, error) { return &gosnmp.SnmpPacket{}, nil },
	}
	collector := New(Config{
		SnmpClient: handler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "present", metrics[0].Tags["static"])
	assert.Equal(t, "fixture", collector.PreparedDeviceMetadata()["vendor"].Value)
	_, refreshErr := collector.RefreshProfileInputs(nil)
	require.Error(t, refreshErr)
	_, err = CollectDeviceMetadata(handler, []*ddsnmp.Profile{profile}, "", logger.New())
	require.NoError(t, err, "standalone metadata acquisition retains its existing response policy")
}

func TestRefreshProfileInputsAcceptsSNMPv1MissingRecovery(t *testing.T) {
	const primary, fallback = "1.2.3.0", "1.2.4.0"
	profile := createTestProfile("device.yaml", nil)
	profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
		"device": {Fields: map[string]ddprofiledefinition.MetadataField{
			"name": {Symbols: []ddprofiledefinition.SymbolConfig{{OID: primary}, {OID: fallback}}},
		}},
	}
	handler := &refreshV1Handler{
		sourceTestHandler: &sourceTestHandler{
			get: func(oids []string) (*gosnmp.SnmpPacket, error) {
				if len(oids) == 2 {
					return &gosnmp.SnmpPacket{
						Error:      gosnmp.NoSuchName,
						ErrorIndex: 1,
					}, nil
				}
				require.Equal(t, []string{fallback}, oids)
				return &gosnmp.SnmpPacket{
					Variables: []gosnmp.SnmpPDU{createStringPDU(fallback, "fallback")},
				}, nil
			},
		},
	}
	collector := New(Config{
		SnmpClient: handler,
		Profiles:   []*ddsnmp.Profile{profile},
		Log:        logger.New(),
	})
	_, refreshErr := collector.RefreshProfileInputs(nil)
	require.NoError(t, refreshErr)
	assert.Equal(t, "fallback", collector.PreparedDeviceMetadata()["name"].Value)
	assert.True(t, collector.missingOIDs[primary])
	assert.Equal(t, 2, handler.gets)
	assert.EqualValues(t, 1, collector.CollectionFailures().GET.Count)
}

func TestProfileInputMetadataFailureClassification(t *testing.T) {
	for name, refresh := range map[string]bool{"ordinary preparation": false, "refresh preparation": true} {
		t.Run(name, func(t *testing.T) {
			profile := createTestProfile("device.yaml", nil)
			profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
				"device": {Fields: map[string]ddprofiledefinition.MetadataField{
					"name": {Symbol: ddprofiledefinition.SymbolConfig{
						OID: "1.2.3.0",
					}},
				}},
			}
			handler := &sourceTestHandler{
				get: func([]string) (*gosnmp.SnmpPacket, error) {
					return nil, gosnmp.ErrWrongDigest
				},
			}
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			var err error
			if refresh {
				_, err = collector.RefreshProfileInputs(nil)
			} else {
				_, err = collector.Collect()
			}
			require.ErrorIs(t, err, gosnmp.ErrWrongDigest)
			failure := snmputils.ClassifyFailure(err)
			assert.Equal(t, "metadata", failure.Operation)
			assert.Equal(t, "wrong_digest", failure.Reason)
		})
	}
}

type refreshV1Handler struct{ *sourceTestHandler }

func (*refreshV1Handler) Version() gosnmp.SnmpVersion { return gosnmp.Version1 }
func (*refreshV1Handler) MaxOids() int                { return 20 }

func TestRefreshProfileInputsAcceptsMetadataFallbackAndAbsence(t *testing.T) {
	const primary, fallback = "1.2.3.0", "1.2.4.0"
	for name, absent := range map[string]bool{"resolved fallback": false, "optional absence": true} {
		t.Run(name, func(t *testing.T) {
			profile := createTestProfile("device.yaml", nil)
			profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
				"device": {Fields: map[string]ddprofiledefinition.MetadataField{
					"vendor": {Value: "fixture"},
					"serial_number": {
						Symbols: []ddprofiledefinition.SymbolConfig{{OID: primary, Format: "hex"}, {OID: fallback}},
					},
				}},
			}
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					oid := oids[0]
					pdu := createStringPDU(oid, "fallback")
					if absent {
						pdu = createNoSuchObjectPDU(oid)
					} else if oid == primary {
						pdu = createIntegerPDU(oid, 42)
					}
					return &gosnmp.SnmpPacket{
						Variables: []gosnmp.SnmpPDU{pdu},
					}, nil
				},
			}
			collector := New(Config{
				SnmpClient: handler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})
			_, refreshErr := collector.RefreshProfileInputs(nil)
			require.NoError(t, refreshErr)
			metadata := collector.PreparedDeviceMetadata()
			assert.Equal(t, "fixture", metadata["vendor"].Value)
			if absent {
				assert.NotContains(t, metadata, "serial_number")
			} else {
				assert.Equal(t, "fallback", metadata["serial_number"].Value)
				assert.EqualValues(t, 1, collector.CollectionFailures().Processing.Preparation)
			}
		})
	}
}

func refreshInputProfile(name, oid string) *ddsnmp.Profile {
	profile := createTestProfile(name, nil)
	profile.Definition.MetricTags = globalMetricTags(ddprofiledefinition.MetricTagConfig{
		Tag: "snmp_host",
		Symbol: ddprofiledefinition.SymbolConfigCompat{
			OID: oid,
		},
	})
	profile.Definition.Metadata = ddprofiledefinition.MetadataConfig{
		"device": {Fields: map[string]ddprofiledefinition.MetadataField{
			"name": {Symbol: ddprofiledefinition.SymbolConfig{
				OID: oid,
			}},
		}},
	}
	return profile
}
