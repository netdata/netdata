// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestRecurringTableEvidence(t *testing.T) {
	const root = "1.3.6.1.4.1.99999.80"
	for name, tc := range map[string]struct{ fallback bool }{
		"cached GET retains original structure":   {},
		"failed candidate GET falls back to WALK": {fallback: true},
	} {
		t.Run(name, func(t *testing.T) {
			handler := &sourceTestHandler{
				walk: func(string) ([]gosnmp.SnmpPDU, error) {
					return []gosnmp.SnmpPDU{createGauge32PDU(root+".1.1", 7)}, nil
				},
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					if tc.fallback {
						return nil, errors.New("candidate timeout")
					}
					return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createGauge32PDU(oids[0], 9)}}, nil
				},
			}
			profile := createTestProfile("table.yaml", []ddprofiledefinition.MetricsConfig{{
				Table:   ddprofiledefinition.SymbolConfig{OID: root, Name: "table"},
				Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}},
			}})
			var reports []AcquisitionProfileReport
			first := &SourceRecorder{ContextID: 1}
			collector := New(Config{SnmpClient: first.Wrap(handler), Profiles: []*ddsnmp.Profile{profile}, Log: logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(func(report AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { reports = append(reports, report) }),
			})
			_, err := collector.Collect()
			require.NoError(t, err)
			firstSources := first.Finish()
			require.Len(t, firstSources, 1)
			require.EqualValues(t, 1, firstSources[0].ContextID)
			require.False(t, firstSources[0].StartedAt.IsZero())
			second := &SourceRecorder{ContextID: 2}
			collector.SetSNMPClient(second.Wrap(handler))
			metrics, err := collector.Collect()
			require.NoError(t, err)
			sources := second.Finish()
			require.Len(t, reports, 2)
			require.Len(t, reports[1].MetricValueReferences, 1)
			require.Len(t, reports[1].Routes, 1)
			route := reports[1].Routes[0]
			var tableInput *AcquisitionCacheInput
			for _, input := range collector.CachedInputs() {
				if input.Kind == "table" {
					tableInput = &input
				}
			}
			require.NotNil(t, tableInput)
			require.Len(t, tableInput.Sources, 1)
			if tc.fallback {
				require.Len(t, sources, 2)
				assert.Equal(t, AcquisitionRouteSourceWalk, route.Source)
				require.Len(t, route.DiscardedSources, 1)
				assert.EqualValues(t, 1, route.DiscardedSources[0].Operation)
				require.Len(t, route.Sources, 1)
				assert.EqualValues(t, 2, route.Sources[0].Operation)
				assert.Same(t, sources[1], tableInput.Sources[0])
				assert.EqualValues(t, 7, metrics[0].Metrics[0].Value)
			} else {
				require.Len(t, sources, 1)
				assert.Equal(t, AcquisitionRouteSourceCache, route.Source)
				assert.Empty(t, route.DiscardedSources)
				require.Len(t, route.Sources, 1)
				assert.EqualValues(t, 1, route.Sources[0].Operation)
				assert.Same(t, firstSources[0], tableInput.Sources[0])
				assert.EqualValues(t, 9, metrics[0].Metrics[0].Value)
			}
			assert.EqualValues(t, 1, firstSources[0].ContextID)
		})
	}
}

func TestRecurringNegativeEvidence(t *testing.T) {
	for name, tc := range map[string]struct{ kind gosnmp.Asn1BER }{
		"no such object":   {gosnmp.NoSuchObject},
		"no such instance": {gosnmp.NoSuchInstance},
	} {
		t.Run(name, func(t *testing.T) {
			const oid = "1.3.6.1.4.1.99999.90.0"
			handler := &sourceTestHandler{get: func([]string) (*gosnmp.SnmpPacket, error) {
				return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: oid, Type: tc.kind}}}, nil
			}}
			profile := createTestProfile("scalar.yaml", []ddprofiledefinition.MetricsConfig{{Symbol: ddprofiledefinition.SymbolConfig{OID: oid, Name: "missing"}}})
			var report AcquisitionProfileReport
			first := &SourceRecorder{ContextID: 11}
			collector := New(Config{SnmpClient: first.Wrap(handler), Profiles: []*ddsnmp.Profile{profile}, Log: logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = value }),
			})
			_, err := collector.Collect()
			require.NoError(t, err)
			first.Finish()
			second := &SourceRecorder{ContextID: 12}
			collector.SetSNMPClient(second.Wrap(handler))
			_, err = collector.Collect()
			require.NoError(t, err)
			assert.Empty(t, second.Finish())
			assert.Equal(t, 1, handler.gets)
			assert.Equal(t, AcquisitionRouteSourceCache, report.Routes[0].Source)
			assert.Equal(t, AcquisitionRouteOutcomeMissing, report.Routes[0].Outcome)
			causes := collector.NegativeCauses()
			require.Len(t, causes, 1)
			assert.EqualValues(t, 11, causes[0].ContextID)
			assert.EqualValues(t, 1, causes[0].Operation)
			assert.Equal(t, uint8(tc.kind), causes[0].PDU.Type)
		})
	}
}

func TestRecurringDependencySettlementEvidence(t *testing.T) {
	for name, tc := range map[string]struct{ dependencyFailure bool }{
		"dependency refresh discards successful candidate": {},
		"failed dependency discards successful candidate":  {true},
	} {
		t.Run(name, func(t *testing.T) {
			dependency, source := crossTableDependencyTestConfigs("1.3.6.1.4.1.99999.81")
			poll := 1
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					if oids[0] == dependency.Symbols[0].OID+".1" {
						return nil, errors.New("dependency cache miss")
					}
					return &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{createGauge32PDU(oids[0], 99)}}, nil
				},
				walk: func(root string) ([]gosnmp.SnmpPDU, error) {
					if poll == 2 && tc.dependencyFailure && root == dependency.Table.OID {
						return nil, errors.New("dependency unavailable")
					}
					return []gosnmp.SnmpPDU{createGauge32PDU(root+".1.1", uint(poll*10))}, nil
				},
			}
			profile := createTestProfile("dependencies.yaml", []ddprofiledefinition.MetricsConfig{source, dependency})
			var report AcquisitionProfileReport
			first := &SourceRecorder{ContextID: 1}
			collector := New(Config{SnmpClient: first.Wrap(handler), Profiles: []*ddsnmp.Profile{profile}, Log: logger.New(),
				AcquisitionObserver: AcquisitionObserverFunc(func(value AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = value }),
			})
			_, err := collector.Collect()
			require.NoError(t, err)
			first.Finish()
			poll = 2
			second := &SourceRecorder{ContextID: 2}
			collector.SetSNMPClient(second.Wrap(handler))
			_, err = collector.Collect()
			require.NoError(t, err)
			operations := second.Finish()
			require.Len(t, operations, 4)
			var sourceRoute *AcquisitionRouteReport
			for i := range report.Routes {
				if report.Routes[i].RootOID == source.Table.OID {
					sourceRoute = &report.Routes[i]
					break
				}
			}
			require.NotNil(t, sourceRoute)
			require.Len(t, sourceRoute.DiscardedSources, 1)
			assert.EqualValues(t, 1, sourceRoute.DiscardedSources[0].Operation)
			for _, binding := range sourceRoute.Sources {
				assert.Greater(t, binding.Operation, uint64(2))
			}
			for _, cache := range collector.CachedInputs() {
				if cache.Kind != "table" {
					continue
				}
				for _, operation := range cache.Sources {
					assert.EqualValues(t, 2, operation.ContextID)
					assert.NotEqual(t, "get", operation.Method)
				}
			}
		})
	}
}

func TestRecurringNegativeEvidenceIgnoresDynamicInstances(t *testing.T) {
	for name, tc := range map[string]struct{ polls int }{"short churn": {3}, "longer churn": {20}} {
		t.Run(name, func(t *testing.T) {
			const root, scalar = "1.3.6.1.4.1.99999.82", "1.3.6.1.4.1.99999.83.0"
			poll := 0
			handler := &sourceTestHandler{
				get: func(oids []string) (*gosnmp.SnmpPacket, error) {
					packet := &gosnmp.SnmpPacket{}
					for _, oid := range oids {
						packet.Variables = append(packet.Variables, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.NoSuchInstance})
					}
					return packet, nil
				},
				walk: func(string) ([]gosnmp.SnmpPDU, error) {
					return []gosnmp.SnmpPDU{createGauge32PDU(fmt.Sprintf("%s.1.%d", root, poll), 7)}, nil
				},
			}
			profile := createTestProfile("churn.yaml", []ddprofiledefinition.MetricsConfig{
				{Symbol: ddprofiledefinition.SymbolConfig{OID: scalar, Name: "unsupported"}},
				{Table: ddprofiledefinition.SymbolConfig{OID: root, Name: "table"}, Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1", Name: "value"}}},
			})
			collector := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
			for poll = 1; poll <= tc.polls; poll++ {
				recorder := &SourceRecorder{ContextID: uint64(poll)}
				collector.SetSNMPClient(recorder.Wrap(handler))
				metrics, err := collector.Collect()
				require.NoError(t, err)
				require.Len(t, metrics[0].Metrics, 1)
				recorder.Finish()
			}
			require.Greater(t, len(collector.missingOIDs), 1, "production cache still records dynamic missing instances")
			causes := collector.NegativeCauses()
			require.Len(t, causes, 1, "only the configured scalar suppresses future acquisition")
			assert.Equal(t, scalar, causes[0].PDU.OID)
			assert.EqualValues(t, 1, causes[0].ContextID)
		})
	}
}
