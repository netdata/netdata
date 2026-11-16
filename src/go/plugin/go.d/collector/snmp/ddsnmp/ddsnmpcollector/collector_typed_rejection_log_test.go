// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const typedRejectionTestRoot = "1.3.6.1.4.1.99999.994"

type typedRejectionTestCase struct {
	components string
	format     string
	badIndex   func(int) string
	goodIndex  string
	goodTag    string
	badValue   bool
	reason     string
}

func typedRejectionTestCases() map[string]typedRejectionTestCase {
	return map[string]typedRejectionTestCase{
		"truncated layout": {
			components: "[{type: octet_string}, {type: integer}]",
			format:     "octet_string",
			badIndex:   func(i int) string { return fmt.Sprintf("3.65.66.%d", i) },
			goodIndex:  "1.71.7",
			goodTag:    "G",
			reason:     "index_decode",
		},
		"invalid octet": {
			components: "[{type: octet_string}, {type: integer}]",
			format:     "octet_string",
			badIndex:   func(i int) string { return fmt.Sprintf("1.256.%d", i) },
			goodIndex:  "1.71.7",
			goodTag:    "G",
			reason:     "index_decode",
		},
		"final formatting": {
			components: "[{type: octet_string}, {type: integer}]",
			format:     "ip_address",
			badIndex:   func(i int) string { return fmt.Sprintf("5.192.0.2.1.9.%d", i) },
			goodIndex:  "4.192.0.2.1.7",
			goodTag:    "192.0.2.1",
			reason:     "IP address",
		},
		"typed signal conversion": {
			components: "[{type: integer}]",
			badIndex:   func(i int) string { return fmt.Sprint(i + 1) },
			goodIndex:  "5000",
			goodTag:    "5000",
			badValue:   true,
			reason:     "not-a-number",
		},
	}
}

func typedRejectionTestProfile(t testing.TB, kind string, tc typedRejectionTestCase) *ddsnmp.Profile {
	t.Helper()
	var row string
	switch kind {
	case "bgp":
		row = `bgp:
- id: rejected-peer-table
  kind: peer
  table: {OID: ` + typedRejectionTestRoot + `, name: peers}
  identity:
    neighbor: {value: "192.0.2.1"}
    remote_as: {value: "65001"}
  transitions:
    established:
      symbol: {OID: ` + typedRejectionTestRoot + `.1, name: transitions}
`
	case "licensing":
		row = `licensing:
- id: license-test
  table: {OID: ` + typedRejectionTestRoot + `, name: licenses}
  identity:
    id: {index: 1}
  signals:
    usage:
      used:
        symbol: {OID: ` + typedRejectionTestRoot + `.1, name: used}
`
	default:
		t.Fatalf("unsupported row kind %q", kind)
	}
	row += fmt.Sprintf(
		"  metric_tags:\n  - tag: name\n    symbol: {format: %q}\n    index_decode: {components: %s, component: 1}\n",
		tc.format,
		tc.components,
	)
	return &ddsnmp.Profile{
		SourceFile: "custom-" + kind + ".yaml",
		Definition: decodeContractProfile(t, row),
	}
}

func typedRejectionTestPDUs(tc typedRejectionTestCase, count int) []gosnmp.SnmpPDU {
	pdus := make([]gosnmp.SnmpPDU, count)
	for i := range pdus {
		oid := typedRejectionTestRoot + ".1." + tc.badIndex(i)
		if tc.badValue {
			pdus[i] = createStringPDU(oid, "not-a-number")
		} else {
			pdus[i] = createCounter32PDU(oid, 10)
		}
	}
	return pdus
}

func TestTypedTableRejectedRowsBoundedDiagnostics(t *testing.T) {
	const count = 4096
	for _, kind := range []string{"bgp", "licensing"} {
		t.Run(kind, func(t *testing.T) {
			for name, tc := range typedRejectionTestCases() {
				t.Run(name, func(t *testing.T) {
					for label, healthy := range map[string]bool{"all rejected": false, "healthy neighbor": true} {
						t.Run(label, func(t *testing.T) {
							pdus := typedRejectionTestPDUs(tc, count)
							goodPDU := createCounter32PDU(typedRejectionTestRoot+".1."+tc.goodIndex, 20)
							if healthy {
								pdus = append(pdus, goodPDU)
							}
							handler := &sourceTestHandler{
								walk: func(oid string) ([]gosnmp.SnmpPDU, error) {
									require.Equal(t, typedRejectionTestRoot, oid)
									return pdus, nil
								},
							}
							var output bytes.Buffer
							var report AcquisitionProfileReport
							collector := New(Config{
								SnmpClient: handler,
								Profiles:   []*ddsnmp.Profile{typedRejectionTestProfile(t, kind, tc)},
								Log:        logger.NewWithWriter(&output),
								AcquisitionObserver: AcquisitionObserverFunc(
									func(r AcquisitionProfileReport, _ *ddsnmp.ProfileMetrics) { report = r },
								),
							})
							expectedRows := 0
							if healthy {
								expectedRows = 1
							}
							initialLogSize := 0
							for cycle := 0; cycle < 2; cycle++ {
								results, err := collector.Collect()
								require.NoError(t, err)
								require.Len(t, results, 1)
								pm := results[0]
								failure := collector.CollectionFailures()
								if kind == "bgp" {
									require.Len(t, pm.BGPRows, expectedRows)
									require.EqualValues(t, count, pm.Stats.Errors.Processing.BGP)
									require.EqualValues(t, count, failure.BGP.Count)
									require.EqualValues(t, count, failure.Processing.BGP)
									require.Equal(t, "bgp", failure.BGP.Last.Operation)
									require.Equal(t, "processing", failure.BGP.Last.Reason)
									if healthy {
										require.NoError(t, pm.BGPCollectError)
										require.Equal(t, tc.goodTag, pm.BGPRows[0].Tags["name"])
										require.EqualValues(t, 20, pm.BGPRows[0].Transitions.Established.Value)
									} else {
										require.Error(t, pm.BGPCollectError)
										diagnostic := pm.BGPCollectError.Error()
										require.Contains(t, diagnostic, "4096")
										require.Contains(t, diagnostic, tc.reason)
										require.Less(t, len(diagnostic), 2048, "returned errors must not retain the row population")
									}
								} else {
									require.Len(t, pm.LicenseRows, expectedRows)
									require.EqualValues(t, count, pm.Stats.Errors.Processing.Licensing)
									require.EqualValues(t, count, failure.Licensing.Count)
									require.EqualValues(t, count, failure.Processing.Licensing)
									require.Equal(t, "licensing", failure.Licensing.Last.Operation)
									require.Equal(t, "processing", failure.Licensing.Last.Reason)
									if healthy {
										require.Equal(t, tc.goodTag, pm.LicenseRows[0].Tags["name"])
										require.True(t, pm.LicenseRows[0].Usage.HasUsed)
										require.EqualValues(t, 20, pm.LicenseRows[0].Usage.Used)
									}
								}
								require.Len(t, report.Routes, 1)
								route := report.Routes[0]
								require.EqualValues(t, len(pdus), route.Rows)
								require.EqualValues(t, expectedRows, route.Values)
								require.Positive(t, route.Rejected)
								require.Equal(t, AcquisitionFailureClassProcessing, route.FailureClass)
								if healthy {
									require.Equal(t, AcquisitionRouteOutcomePartial, route.Outcome)
									require.Empty(
										t,
										output.String(),
										"partial table success retains its existing quiet diagnostic behavior",
									)
								} else {
									require.Equal(t, AcquisitionRouteOutcomeRejected, route.Outcome)
									require.Less(t, output.Len(), 4096, "warning must summarize the rejected population")
									require.Contains(t, output.String(), "4096")
									require.Contains(t, output.String(), tc.reason)
									if cycle == 0 {
										initialLogSize = output.Len()
									} else {
										require.Equal(t, initialLogSize, output.Len(), "the existing limiter remains effective across polls")
									}
								}
							}
							// Fresh typed-row acquisition recovers without changing the profile, collector or logger.
							pdus = []gosnmp.SnmpPDU{goodPDU}
							results, err := collector.Collect()
							require.NoError(t, err)
							require.Len(t, results, 1)
							pm := results[0]
							require.NoError(t, pm.BGPCollectError)
							require.Zero(t, pm.Stats.Errors.Processing.BGP)
							require.Zero(t, pm.Stats.Errors.Processing.Licensing)
							require.Zero(t, collector.CollectionFailures().BGP.Count)
							require.Zero(t, collector.CollectionFailures().Licensing.Count)
							if kind == "bgp" {
								require.Len(t, pm.BGPRows, 1)
								require.Equal(t, tc.goodTag, pm.BGPRows[0].Tags["name"])
							} else {
								require.Len(t, pm.LicenseRows, 1)
								require.Equal(t, tc.goodTag, pm.LicenseRows[0].Tags["name"])
							}
							require.Len(t, report.Routes, 1)
							require.Equal(t, AcquisitionRouteOutcomeValues, report.Routes[0].Outcome)
							require.Zero(t, report.Routes[0].Rejected)
							require.Equal(t, initialLogSize, output.Len())
							require.Equal(t, 3, handler.walks, "typed rows remain fresh on every poll")
						})
					}
				})
			}
		})
	}
}

// Diagnostic retention is O(configured tables), with one representative row error per table.
// Rejection still performs O(rows) decoding; ns/op is only a local trend indicator.
func BenchmarkTypedTableRejectedRows(b *testing.B) {
	for _, kind := range []string{"bgp", "licensing"} {
		for _, count := range []int{1, 4096} {
			b.Run(fmt.Sprintf("%s/%d", kind, count), func(b *testing.B) {
				tc := typedRejectionTestCases()["invalid octet"]
				pdus := typedRejectionTestPDUs(tc, count)
				handler := &sourceTestHandler{
					walk: func(string) ([]gosnmp.SnmpPDU, error) { return pdus, nil },
				}
				collector := New(
					Config{
						SnmpClient: handler,
						Profiles:   []*ddsnmp.Profile{typedRejectionTestProfile(b, kind, tc)},
						Log:        logger.NewWithWriter(io.Discard),
					},
				)
				_, err := collector.Collect()
				require.NoError(b, err)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := collector.Collect(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
