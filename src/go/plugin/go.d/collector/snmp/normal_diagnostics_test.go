// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

const normalTestRoot = "1.3.6.1.4.1.99999.100"

func normalTestProfile() *ddsnmp.Profile {
	return &ddsnmp.Profile{SourceFile: "normal.yaml", Definition: &ddprofiledefinition.ProfileDefinition{
		Metrics: []ddprofiledefinition.MetricsConfig{
			{Symbol: ddprofiledefinition.SymbolConfig{OID: normalTestRoot + ".1.0", Name: "visible"}},
			{Symbol: ddprofiledefinition.SymbolConfig{OID: normalTestRoot + ".2.0", Name: "_hidden"}},
		},
		VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{{Name: "derived", Sources: []ddprofiledefinition.VirtualMetricSourceConfig{{Metric: "_hidden"}}}},
		Licensing: []ddprofiledefinition.LicensingConfig{{
			ID: "license", OriginProfileID: "normal.yaml",
			State:   ddprofiledefinition.LicenseStateConfig{LicenseValueConfig: ddprofiledefinition.LicenseValueConfig{OID: normalTestRoot + ".3.0"}},
			Signals: ddprofiledefinition.LicenseSignalsConfig{Expiry: ddprofiledefinition.LicenseTimerSignalsConfig{Remaining: ddprofiledefinition.LicenseValueConfig{OID: normalTestRoot + ".4.0"}}},
		}},
		BGP: []ddprofiledefinition.BGPConfig{{
			ID: "peer", OriginProfileID: "normal.yaml", Kind: ddprofiledefinition.BGPRowKindPeer,
			Identity: ddprofiledefinition.BGPIdentityConfig{Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.2"}, RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"}},
			State: ddprofiledefinition.BGPStateConfig{BGPValueConfig: ddprofiledefinition.BGPValueConfig{Symbol: ddprofiledefinition.SymbolConfig{
				OID: normalTestRoot + ".5.0", Name: "state", Mapping: ddprofiledefinition.NewExactMapping(map[string]string{"6": "established"}),
			}}},
		}},
	}}
}

func normalTestDocument(t *testing.T, c *Collector) *diagnostics.NormalDevice {
	t.Helper()
	device, err := c.normal.cut.CaptureNormal()
	require.NoError(t, err)
	device.RegistrationID, device.RuntimeID = 1, 1
	require.NoError(t, device.Validate())
	var archive bytes.Buffer
	require.NoError(t, diagnostics.Write(&archive, diagnostics.Document{Format: diagnostics.Format, Version: diagnostics.Version, Kind: diagnostics.KindNormal, Normal: device}))
	document, err := diagnostics.Read(&archive, diagnostics.DefaultReadLimits())
	require.NoError(t, err)
	require.NoError(t, document.Normal.Validate())
	return document.Normal
}

func TestNormalEvidenceRealCollection(t *testing.T) {
	for name, tc := range map[string]struct{ failure string }{
		"total collection failure":                 {"metrics"},
		"malformed BGP value":                      {"bgp_conversion"},
		"absent BGP table signals":                 {"missing_bgp_table"},
		"absent BGP identity":                      {"missing_bgp_identity"},
		"partial BGP failure":                      {"bgp"},
		"malformed licensing timer":                {"license"},
		"absent BGP signals with available tags":   {"missing_bgp_tags"},
		"ordinary absent BGP signals":              {"missing_bgp"},
		"ordinary absent timer":                    {"missing"},
		"cached rejected tag is not a new failure": {"cached"},
	} {
		t.Run(name, func(t *testing.T) {
			bgpMissing := strings.HasPrefix(tc.failure, "missing_bgp")
			handler, cleanup := mockInit(t)
			defer cleanup()
			handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
			setMockClientInitExpect(handler)
			handler.EXPECT().BulkWalkAll(snmputils.RootOidMibSystem).Return([]gosnmp.SnmpPDU{{Name: snmputils.OidSysName, Type: gosnmp.OctetString, Value: "switch"}}, nil).Times(2)
			handler.EXPECT().MaxOids().Return(20).AnyTimes()
			poll := 0
			handler.EXPECT().Get(gomock.Any()).DoAndReturn(func(oids []string) (*gosnmp.SnmpPacket, error) {
				packet := &gosnmp.SnmpPacket{}
				for _, oid := range oids {
					if oid == snmputils.OidSysObject {
						packet.Variables = append(packet.Variables, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.99999"})
						continue
					}
					if poll == 2 && (((tc.failure == "metrics" || bgpMissing) && oid == normalTestRoot+".1.0") || (tc.failure == "bgp" && oid == normalTestRoot+".5.0")) {
						return nil, errors.New("synthetic timeout")
					}
					value := 7
					if oid == normalTestRoot+".3.0" {
						value = 0
					}
					if oid == normalTestRoot+".4.0" {
						value = 3600
					}
					if oid == normalTestRoot+".5.0" {
						value = 6
					}
					pdu := gosnmp.SnmpPDU{Name: oid, Type: gosnmp.Integer, Value: value}
					if poll == 2 && tc.failure == "bgp_conversion" && oid == normalTestRoot+".7.0" {
						pdu.Type, pdu.Value = gosnmp.OctetString, "invalid"
					}
					missingBGPOID := normalTestRoot + ".5.0"
					if tc.failure == "missing_bgp_identity" {
						missingBGPOID = normalTestRoot + ".6.0"
					}
					if bgpMissing && oid == missingBGPOID {
						pdu.Type, pdu.Value = gosnmp.NoSuchObject, nil
					}
					if tc.failure == "cached" && oid == normalTestRoot+".6.0" {
						pdu.Type, pdu.Value = gosnmp.OctetString, "invalid"
					}
					if poll == 2 && oid == normalTestRoot+".4.0" {
						if tc.failure == "license" {
							pdu.Type, pdu.Value = gosnmp.OctetString, "not-a-number"
						}
						if tc.failure == "missing" {
							pdu.Type, pdu.Value = gosnmp.NoSuchInstance, nil
						}
					}
					packet.Variables = append(packet.Variables, pdu)
				}
				return packet, nil
			}).AnyTimes()
			store := ddsnmp.NewDeviceStore()
			root := t.TempDir()
			directory := diagnostics.DirectoryPath(root)
			publisher := diagnostics.NewPublisher(store, root)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); publisher.Run(ctx) }()
			t.Cleanup(func() { cancel(); <-done })
			creator := Creator(store, publisher)
			c := creator.Create().(*Collector)
			c.Config = prepareV2Config()
			c.newSnmpClient = func() gosnmp.Handler { return handler }
			// Inject profile input only; Init, Check, all acquisition, normalization
			// and consumer commits run through their production implementations.
			c.snmpProfiles = []*ddsnmp.Profile{normalTestProfile()}
			if tc.failure == "bgp_conversion" {
				c.snmpProfiles[0].Definition.BGP[0].Connection.EstablishedUptime = ddprofiledefinition.BGPValueConfig{Symbol: ddprofiledefinition.SymbolConfig{OID: normalTestRoot + ".7.0", Name: "uptime"}}
			}
			if tc.failure == "missing_bgp_identity" {
				c.snmpProfiles[0].Definition.BGP[0].Identity.Neighbor = ddprofiledefinition.BGPValueConfig{Symbol: ddprofiledefinition.SymbolConfig{OID: normalTestRoot + ".6.0", Name: "neighbor"}}
			}
			if tc.failure == "missing_bgp_table" {
				cfg := &c.snmpProfiles[0].Definition.BGP[0]
				root := normalTestRoot + ".20"
				cfg.Table = ddprofiledefinition.SymbolConfig{OID: root, Name: "peers"}
				cfg.State.Symbol.OID = root + ".1"
				cfg.MetricTags = []ddprofiledefinition.MetricTagConfig{{Tag: "site", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".2", Name: "site"}}}
				handler.EXPECT().BulkWalkAll(root).Return([]gosnmp.SnmpPDU{{Name: root + ".2.1", Type: gosnmp.OctetString, Value: "site"}}, nil).AnyTimes()
			}
			if tc.failure == "missing_bgp_tags" {
				c.snmpProfiles[0].Definition.BGP[0].MetricTags = []ddprofiledefinition.MetricTagConfig{{Tag: "site", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: normalTestRoot + ".6.0", Name: "site"}}}
			}
			if tc.failure == "cached" {
				c.snmpProfiles[0].Definition.MetricTags = []ddprofiledefinition.GlobalMetricTagConfig{{MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "site", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: normalTestRoot + ".6.0", Name: "site", ExtractValue: "^([0-9]+)$", ExtractValueCompiled: regexp.MustCompile("^([0-9]+)$")}}}}
			}
			require.NoError(t, ddsnmp.CompileTransforms(c.snmpProfiles[0]))
			job := snmpLifecycleTestRuntimeJob{collector: c}
			identity := collectorapi.JobConfigIdentity{1}
			creator.JobConfigLifecycle.Bind(identity, job)
			require.NoError(t, c.Init(t.Context()))
			require.NoError(t, c.Check(t.Context()))
			files, err := diagnostics.ListNormalFiles(directory)
			require.NoError(t, err)
			assert.Empty(t, files, "candidate evidence is not published before acceptance")
			creator.JobConfigLifecycle.Reconcile(collectorapi.JobConfigIdentity{}, creator.JobConfigLifecycle.Capture(identity, job), job)
			poll = 1
			firstSamples := c.Collect(t.Context())
			require.NotEmpty(t, firstSamples)
			first := normalTestDocument(t, c)
			require.Equal(t, tc.failure == "cached", first.Latest.Failed)
			if bgpMissing {
				require.Empty(t, first.Latest.BGP.Entries)
			} else {
				require.Len(t, first.Latest.BGP.Entries, 1)
			}
			require.Len(t, first.Latest.Licensing.Rows, 1)
			require.NotEmpty(t, first.Initialization)
			require.Len(t, first.Latest.Profiles[0].HiddenMetrics, 1)
			var virtual bool
			for _, metric := range first.Latest.Profiles[0].Metrics {
				if metric.Name == "derived" {
					virtual = metric.IsVirtual
				}
			}
			assert.True(t, virtual)
			assert.NotContains(t, firstSamples, metricIDFromName("_hidden"))
			poll = 2
			secondSamples := c.Collect(t.Context())
			second := normalTestDocument(t, c)
			assert.Equal(t, tc.failure != "missing" && tc.failure != "cached", second.Latest.Failed)
			if tc.failure == "metrics" {
				assert.Empty(t, secondSamples)
				assert.Equal(t, first.Latest.Licensing.NormalizedAt, second.Latest.Licensing.NormalizedAt)
				assert.Equal(t, first.Latest.Licensing.Sources, second.Latest.Licensing.Sources)
				assert.True(t, c.bgp.peerCache.snapshot(time.Now().Add(c.bgpStaleAfter())).expired)
			}
			if tc.failure == "metrics" || tc.failure == "bgp" {
				assert.Equal(t, first.Latest.BGP.Sources, second.Latest.BGP.Sources)
				require.Len(t, second.Latest.BGP.Entries, 1)
				assert.Equal(t, first.Latest.BGP.Entries[0].LastUpdate, second.Latest.BGP.Entries[0].LastUpdate)
			}
			poll = 3
			require.NotEmpty(t, c.Collect(t.Context()))
			third := normalTestDocument(t, c)
			assert.False(t, third.Latest.Failed)
			if tc.failure != "missing" {
				require.NotNil(t, third.LastFailure)
				wantFailure := second.Latest.ID
				if tc.failure == "cached" {
					wantFailure = first.Latest.ID
				}
				assert.Equal(t, wantFailure, third.LastFailure.ID)
			} else {
				assert.Nil(t, third.LastFailure)
			}
			assert.Equal(t, firstSamples, first.Latest.Samples, "retained first document remains independent")
			cancel()
			<-done
			require.NoError(t, publisher.Finalize(t.Context()))
			files, err = diagnostics.ListNormalFiles(directory)
			require.NoError(t, err)
			require.Len(t, files, 1)
			data, err := os.ReadFile(filepath.Join(directory, diagnostics.NormalDirectory, files[0].RunID, files[0].Filename))
			require.NoError(t, err)
			published, err := diagnostics.Read(bytes.NewReader(data), diagnostics.DefaultReadLimits())
			require.NoError(t, err)
			require.NoError(t, published.Normal.Validate())
			assert.Equal(t, third.Latest, published.Normal.Latest)
			assert.Equal(t, third.LastFailure, published.Normal.LastFailure)
			creator.JobConfigLifecycle.Remove(identity)
			files, err = diagnostics.ListNormalFiles(directory)
			require.NoError(t, err)
			assert.Len(t, files, 1, "shutdown retirement preserves the last published file")
		})
	}
}

func TestNormalMetricSampleOrder(t *testing.T) {
	for name, tc := range map[string]struct{ table bool }{
		"scalar": {}, "table": {table: true},
	} {
		t.Run(name, func(t *testing.T) {
			c := New(ddsnmp.NewDeviceStore())
			c.sysInfo = &snmputils.SysInfo{Name: "switch"}
			c.beginNormalAttempt("collect")
			metric := ddsnmp.Metric{
				Profile: &ddsnmp.ProfileMetrics{Source: "states.yaml"}, Name: "state", IsTable: tc.table, Table: "states", Tags: map[string]string{"index": "1"},
				MultiValue: make(map[string]int64),
			}
			for i := range 16 {
				metric.MultiValue[fmt.Sprintf("state%02d", i)] = int64(i)
			}
			samples := make(map[string]int64)
			metrics := []ddsnmp.Metric{metric}
			c.collectProfileScalarMetrics(samples, metrics)
			c.collectProfileTableMetrics(samples, metrics)
			require.Len(t, c.normal.current.document.Metrics, 1)
			decision := c.normal.current.document.Metrics[0]
			require.Len(t, decision.SampleIDs, len(metric.MultiValue))
			require.True(t, slices.IsSorted(decision.SampleIDs), "%v", decision.SampleIDs)
			require.Len(t, samples, len(metric.MultiValue))
			for key, value := range metric.MultiValue {
				id := metricIDFromName(metric.Name, key)
				if tc.table {
					id = metricIDFromKey(tableMetricKey(metric), key)
				}
				require.Contains(t, decision.SampleIDs, id)
				require.Equal(t, value, samples[id])
			}
		})
	}
}
