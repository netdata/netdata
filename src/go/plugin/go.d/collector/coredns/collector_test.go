// SPDX-License-Identifier: GPL-3.0-or-later

package coredns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer169NoLoad, _   = os.ReadFile("testdata/version169/no_load.txt")
	dataVer169SomeLoad, _ = os.ReadFile("testdata/version169/some_load.txt")

	dataVer170NoLoad, _   = os.ReadFile("testdata/version170/no_load.txt")
	dataVer170SomeLoad, _ = os.ReadFile("testdata/version170/some_load.txt")

	dataNoLoadNoVersion, _ = os.ReadFile("testdata/no_version/no_load.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataVer169NoLoad":    dataVer169NoLoad,
		"dataVer169SomeLoad":  dataVer169SomeLoad,
		"dataVer170NoLoad":    dataVer170NoLoad,
		"dataVer170SomeLoad":  dataVer170SomeLoad,
		"dataNoLoadNoVersion": dataNoLoadNoVersion,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Init(t *testing.T) {
	assert.NoError(t, New().Init(context.Background()))
}

func TestCollector_InitNG(t *testing.T) {
	collr := New()
	collr.URL = ""
	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"version 1.6.9", dataVer169NoLoad},
		{"version 1.7.0", dataVer170NoLoad},
	}
	for _, testNoLoad := range tests {
		t.Run(testNoLoad.name, func(t *testing.T) {

			ts := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write(testNoLoad.data)
					}))
			defer ts.Close()

			collr := New()
			collr.URL = ts.URL + "/metrics"
			require.NoError(t, collr.Init(context.Background()))
			assert.NoError(t, collr.Check(context.Background()))
		})
	}
}

func TestCollector_CheckNG(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"version 1.6.9", dataVer169SomeLoad},
		{"version 1.7.0", dataVer170SomeLoad},
	}
	for _, testSomeLoad := range tests {
		t.Run(testSomeLoad.name, func(t *testing.T) {

			ts := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write(testSomeLoad.data)
					}))
			defer ts.Close()

			collr := New()
			collr.URL = ts.URL + "/metrics"
			collr.PerServerStats.Includes = []string{"glob:*"}
			collr.PerZoneStats.Includes = []string{"glob:*"}
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))

			expected := map[string]int64{
				"coredns.io._request_per_ip_family_v4":     19,
				"coredns.io._request_per_ip_family_v6":     0,
				"coredns.io._request_per_proto_tcp":        0,
				"coredns.io._request_per_proto_udp":        19,
				"coredns.io._request_per_status_dropped":   0,
				"coredns.io._request_per_status_processed": 0,
				"coredns.io._request_per_type_A":           6,
				"coredns.io._request_per_type_AAAA":        6,
				"coredns.io._request_per_type_ANY":         0,
				"coredns.io._request_per_type_CNAME":       0,
				"coredns.io._request_per_type_DNSKEY":      0,
				"coredns.io._request_per_type_DS":          0,
				"coredns.io._request_per_type_IXFR":        0,
				"coredns.io._request_per_type_MX":          7,
				"coredns.io._request_per_type_NS":          0,
				"coredns.io._request_per_type_NSEC":        0,
				"coredns.io._request_per_type_NSEC3":       0,
				"coredns.io._request_per_type_PTR":         0,
				"coredns.io._request_per_type_RRSIG":       0,
				"coredns.io._request_per_type_SOA":         0,
				"coredns.io._request_per_type_SRV":         0,
				"coredns.io._request_per_type_TXT":         0,
				"coredns.io._request_per_type_other":       0,
				"coredns.io._request_total":                19,
				"coredns.io._response_per_rcode_BADALG":    0,
				"coredns.io._response_per_rcode_BADCOOKIE": 0,
				"coredns.io._response_per_rcode_BADKEY":    0,
				"coredns.io._response_per_rcode_BADMODE":   0,
				"coredns.io._response_per_rcode_BADNAME":   0,
				"coredns.io._response_per_rcode_BADSIG":    0,
				"coredns.io._response_per_rcode_BADTIME":   0,
				"coredns.io._response_per_rcode_BADTRUNC":  0,
				"coredns.io._response_per_rcode_FORMERR":   0,
				"coredns.io._response_per_rcode_NOERROR":   19,
				"coredns.io._response_per_rcode_NOTAUTH":   0,
				"coredns.io._response_per_rcode_NOTIMP":    0,
				"coredns.io._response_per_rcode_NOTZONE":   0,
				"coredns.io._response_per_rcode_NXDOMAIN":  0,
				"coredns.io._response_per_rcode_NXRRSET":   0,
				"coredns.io._response_per_rcode_REFUSED":   0,
				"coredns.io._response_per_rcode_SERVFAIL":  0,
				"coredns.io._response_per_rcode_YXDOMAIN":  0,
				"coredns.io._response_per_rcode_YXRRSET":   0,
				"coredns.io._response_per_rcode_other":     0,
				"coredns.io._response_total":               19,
				"dns://:53_request_per_ip_family_v4":       15,
				"dns://:53_request_per_ip_family_v6":       0,
				"dns://:53_request_per_proto_tcp":          0,
				"dns://:53_request_per_proto_udp":          15,
				"dns://:53_request_per_status_dropped":     9,
				"dns://:53_request_per_status_processed":   6,
				"dns://:53_request_per_type_A":             5,
				"dns://:53_request_per_type_AAAA":          5,
				"dns://:53_request_per_type_ANY":           0,
				"dns://:53_request_per_type_CNAME":         0,
				"dns://:53_request_per_type_DNSKEY":        0,
				"dns://:53_request_per_type_DS":            0,
				"dns://:53_request_per_type_IXFR":          0,
				"dns://:53_request_per_type_MX":            5,
				"dns://:53_request_per_type_NS":            0,
				"dns://:53_request_per_type_NSEC":          0,
				"dns://:53_request_per_type_NSEC3":         0,
				"dns://:53_request_per_type_PTR":           0,
				"dns://:53_request_per_type_RRSIG":         0,
				"dns://:53_request_per_type_SOA":           0,
				"dns://:53_request_per_type_SRV":           0,
				"dns://:53_request_per_type_TXT":           0,
				"dns://:53_request_per_type_other":         0,
				"dns://:53_request_total":                  15,
				"dns://:53_response_per_rcode_BADALG":      0,
				"dns://:53_response_per_rcode_BADCOOKIE":   0,
				"dns://:53_response_per_rcode_BADKEY":      0,
				"dns://:53_response_per_rcode_BADMODE":     0,
				"dns://:53_response_per_rcode_BADNAME":     0,
				"dns://:53_response_per_rcode_BADSIG":      0,
				"dns://:53_response_per_rcode_BADTIME":     0,
				"dns://:53_response_per_rcode_BADTRUNC":    0,
				"dns://:53_response_per_rcode_FORMERR":     0,
				"dns://:53_response_per_rcode_NOERROR":     6,
				"dns://:53_response_per_rcode_NOTAUTH":     0,
				"dns://:53_response_per_rcode_NOTIMP":      0,
				"dns://:53_response_per_rcode_NOTZONE":     0,
				"dns://:53_response_per_rcode_NXDOMAIN":    0,
				"dns://:53_response_per_rcode_NXRRSET":     0,
				"dns://:53_response_per_rcode_REFUSED":     0,
				"dns://:53_response_per_rcode_SERVFAIL":    9,
				"dns://:53_response_per_rcode_YXDOMAIN":    0,
				"dns://:53_response_per_rcode_YXRRSET":     0,
				"dns://:53_response_per_rcode_other":       0,
				"dns://:53_response_total":                 15,
				"dns://:54_request_per_ip_family_v4":       25,
				"dns://:54_request_per_ip_family_v6":       0,
				"dns://:54_request_per_proto_tcp":          0,
				"dns://:54_request_per_proto_udp":          25,
				"dns://:54_request_per_status_dropped":     12,
				"dns://:54_request_per_status_processed":   13,
				"dns://:54_request_per_type_A":             8,
				"dns://:54_request_per_type_AAAA":          8,
				"dns://:54_request_per_type_ANY":           0,
				"dns://:54_request_per_type_CNAME":         0,
				"dns://:54_request_per_type_DNSKEY":        0,
				"dns://:54_request_per_type_DS":            0,
				"dns://:54_request_per_type_IXFR":          0,
				"dns://:54_request_per_type_MX":            9,
				"dns://:54_request_per_type_NS":            0,
				"dns://:54_request_per_type_NSEC":          0,
				"dns://:54_request_per_type_NSEC3":         0,
				"dns://:54_request_per_type_PTR":           0,
				"dns://:54_request_per_type_RRSIG":         0,
				"dns://:54_request_per_type_SOA":           0,
				"dns://:54_request_per_type_SRV":           0,
				"dns://:54_request_per_type_TXT":           0,
				"dns://:54_request_per_type_other":         0,
				"dns://:54_request_total":                  25,
				"dns://:54_response_per_rcode_BADALG":      0,
				"dns://:54_response_per_rcode_BADCOOKIE":   0,
				"dns://:54_response_per_rcode_BADKEY":      0,
				"dns://:54_response_per_rcode_BADMODE":     0,
				"dns://:54_response_per_rcode_BADNAME":     0,
				"dns://:54_response_per_rcode_BADSIG":      0,
				"dns://:54_response_per_rcode_BADTIME":     0,
				"dns://:54_response_per_rcode_BADTRUNC":    0,
				"dns://:54_response_per_rcode_FORMERR":     0,
				"dns://:54_response_per_rcode_NOERROR":     13,
				"dns://:54_response_per_rcode_NOTAUTH":     0,
				"dns://:54_response_per_rcode_NOTIMP":      0,
				"dns://:54_response_per_rcode_NOTZONE":     0,
				"dns://:54_response_per_rcode_NXDOMAIN":    0,
				"dns://:54_response_per_rcode_NXRRSET":     0,
				"dns://:54_response_per_rcode_REFUSED":     0,
				"dns://:54_response_per_rcode_SERVFAIL":    12,
				"dns://:54_response_per_rcode_YXDOMAIN":    0,
				"dns://:54_response_per_rcode_YXRRSET":     0,
				"dns://:54_response_per_rcode_other":       0,
				"dns://:54_response_total":                 25,
				"dropped_request_per_ip_family_v4":         42,
				"dropped_request_per_ip_family_v6":         0,
				"dropped_request_per_proto_tcp":            0,
				"dropped_request_per_proto_udp":            42,
				"dropped_request_per_status_dropped":       0,
				"dropped_request_per_status_processed":     0,
				"dropped_request_per_type_A":               14,
				"dropped_request_per_type_AAAA":            14,
				"dropped_request_per_type_ANY":             0,
				"dropped_request_per_type_CNAME":           0,
				"dropped_request_per_type_DNSKEY":          0,
				"dropped_request_per_type_DS":              0,
				"dropped_request_per_type_IXFR":            0,
				"dropped_request_per_type_MX":              14,
				"dropped_request_per_type_NS":              0,
				"dropped_request_per_type_NSEC":            0,
				"dropped_request_per_type_NSEC3":           0,
				"dropped_request_per_type_PTR":             0,
				"dropped_request_per_type_RRSIG":           0,
				"dropped_request_per_type_SOA":             0,
				"dropped_request_per_type_SRV":             0,
				"dropped_request_per_type_TXT":             0,
				"dropped_request_per_type_other":           0,
				"dropped_request_total":                    42,
				"dropped_response_per_rcode_BADALG":        0,
				"dropped_response_per_rcode_BADCOOKIE":     0,
				"dropped_response_per_rcode_BADKEY":        0,
				"dropped_response_per_rcode_BADMODE":       0,
				"dropped_response_per_rcode_BADNAME":       0,
				"dropped_response_per_rcode_BADSIG":        0,
				"dropped_response_per_rcode_BADTIME":       0,
				"dropped_response_per_rcode_BADTRUNC":      0,
				"dropped_response_per_rcode_FORMERR":       0,
				"dropped_response_per_rcode_NOERROR":       0,
				"dropped_response_per_rcode_NOTAUTH":       0,
				"dropped_response_per_rcode_NOTIMP":        0,
				"dropped_response_per_rcode_NOTZONE":       0,
				"dropped_response_per_rcode_NXDOMAIN":      0,
				"dropped_response_per_rcode_NXRRSET":       0,
				"dropped_response_per_rcode_REFUSED":       21,
				"dropped_response_per_rcode_SERVFAIL":      21,
				"dropped_response_per_rcode_YXDOMAIN":      0,
				"dropped_response_per_rcode_YXRRSET":       0,
				"dropped_response_per_rcode_other":         0,
				"dropped_response_total":                   42,
				"empty_request_per_ip_family_v4":           21,
				"empty_request_per_ip_family_v6":           0,
				"empty_request_per_proto_tcp":              0,
				"empty_request_per_proto_udp":              21,
				"empty_request_per_status_dropped":         21,
				"empty_request_per_status_processed":       0,
				"empty_request_per_type_A":                 7,
				"empty_request_per_type_AAAA":              7,
				"empty_request_per_type_ANY":               0,
				"empty_request_per_type_CNAME":             0,
				"empty_request_per_type_DNSKEY":            0,
				"empty_request_per_type_DS":                0,
				"empty_request_per_type_IXFR":              0,
				"empty_request_per_type_MX":                7,
				"empty_request_per_type_NS":                0,
				"empty_request_per_type_NSEC":              0,
				"empty_request_per_type_NSEC3":             0,
				"empty_request_per_type_PTR":               0,
				"empty_request_per_type_RRSIG":             0,
				"empty_request_per_type_SOA":               0,
				"empty_request_per_type_SRV":               0,
				"empty_request_per_type_TXT":               0,
				"empty_request_per_type_other":             0,
				"empty_request_total":                      21,
				"empty_response_per_rcode_BADALG":          0,
				"empty_response_per_rcode_BADCOOKIE":       0,
				"empty_response_per_rcode_BADKEY":          0,
				"empty_response_per_rcode_BADMODE":         0,
				"empty_response_per_rcode_BADNAME":         0,
				"empty_response_per_rcode_BADSIG":          0,
				"empty_response_per_rcode_BADTIME":         0,
				"empty_response_per_rcode_BADTRUNC":        0,
				"empty_response_per_rcode_FORMERR":         0,
				"empty_response_per_rcode_NOERROR":         0,
				"empty_response_per_rcode_NOTAUTH":         0,
				"empty_response_per_rcode_NOTIMP":          0,
				"empty_response_per_rcode_NOTZONE":         0,
				"empty_response_per_rcode_NXDOMAIN":        0,
				"empty_response_per_rcode_NXRRSET":         0,
				"empty_response_per_rcode_REFUSED":         21,
				"empty_response_per_rcode_SERVFAIL":        0,
				"empty_response_per_rcode_YXDOMAIN":        0,
				"empty_response_per_rcode_YXRRSET":         0,
				"empty_response_per_rcode_other":           0,
				"empty_response_total":                     21,
				"no_matching_zone_dropped_total":           21,
				"panic_total":                              0,
				"request_per_ip_family_v4":                 61,
				"request_per_ip_family_v6":                 0,
				"request_per_proto_tcp":                    0,
				"request_per_proto_udp":                    61,
				"request_per_status_dropped":               42,
				"request_per_status_processed":             19,
				"request_per_type_A":                       20,
				"request_per_type_AAAA":                    20,
				"request_per_type_ANY":                     0,
				"request_per_type_CNAME":                   0,
				"request_per_type_DNSKEY":                  0,
				"request_per_type_DS":                      0,
				"request_per_type_IXFR":                    0,
				"request_per_type_MX":                      21,
				"request_per_type_NS":                      0,
				"request_per_type_NSEC":                    0,
				"request_per_type_NSEC3":                   0,
				"request_per_type_PTR":                     0,
				"request_per_type_RRSIG":                   0,
				"request_per_type_SOA":                     0,
				"request_per_type_SRV":                     0,
				"request_per_type_TXT":                     0,
				"request_per_type_other":                   0,
				"request_total":                            61,
				"response_per_rcode_BADALG":                0,
				"response_per_rcode_BADCOOKIE":             0,
				"response_per_rcode_BADKEY":                0,
				"response_per_rcode_BADMODE":               0,
				"response_per_rcode_BADNAME":               0,
				"response_per_rcode_BADSIG":                0,
				"response_per_rcode_BADTIME":               0,
				"response_per_rcode_BADTRUNC":              0,
				"response_per_rcode_FORMERR":               0,
				"response_per_rcode_NOERROR":               19,
				"response_per_rcode_NOTAUTH":               0,
				"response_per_rcode_NOTIMP":                0,
				"response_per_rcode_NOTZONE":               0,
				"response_per_rcode_NXDOMAIN":              0,
				"response_per_rcode_NXRRSET":               0,
				"response_per_rcode_REFUSED":               21,
				"response_per_rcode_SERVFAIL":              21,
				"response_per_rcode_YXDOMAIN":              0,
				"response_per_rcode_YXRRSET":               0,
				"response_per_rcode_other":                 0,
				"response_total":                           61,
				"ya.ru._request_per_ip_family_v4":          21,
				"ya.ru._request_per_ip_family_v6":          0,
				"ya.ru._request_per_proto_tcp":             0,
				"ya.ru._request_per_proto_udp":             21,
				"ya.ru._request_per_status_dropped":        0,
				"ya.ru._request_per_status_processed":      0,
				"ya.ru._request_per_type_A":                7,
				"ya.ru._request_per_type_AAAA":             7,
				"ya.ru._request_per_type_ANY":              0,
				"ya.ru._request_per_type_CNAME":            0,
				"ya.ru._request_per_type_DNSKEY":           0,
				"ya.ru._request_per_type_DS":               0,
				"ya.ru._request_per_type_IXFR":             0,
				"ya.ru._request_per_type_MX":               7,
				"ya.ru._request_per_type_NS":               0,
				"ya.ru._request_per_type_NSEC":             0,
				"ya.ru._request_per_type_NSEC3":            0,
				"ya.ru._request_per_type_PTR":              0,
				"ya.ru._request_per_type_RRSIG":            0,
				"ya.ru._request_per_type_SOA":              0,
				"ya.ru._request_per_type_SRV":              0,
				"ya.ru._request_per_type_TXT":              0,
				"ya.ru._request_per_type_other":            0,
				"ya.ru._request_total":                     21,
				"ya.ru._response_per_rcode_BADALG":         0,
				"ya.ru._response_per_rcode_BADCOOKIE":      0,
				"ya.ru._response_per_rcode_BADKEY":         0,
				"ya.ru._response_per_rcode_BADMODE":        0,
				"ya.ru._response_per_rcode_BADNAME":        0,
				"ya.ru._response_per_rcode_BADSIG":         0,
				"ya.ru._response_per_rcode_BADTIME":        0,
				"ya.ru._response_per_rcode_BADTRUNC":       0,
				"ya.ru._response_per_rcode_FORMERR":        0,
				"ya.ru._response_per_rcode_NOERROR":        0,
				"ya.ru._response_per_rcode_NOTAUTH":        0,
				"ya.ru._response_per_rcode_NOTIMP":         0,
				"ya.ru._response_per_rcode_NOTZONE":        0,
				"ya.ru._response_per_rcode_NXDOMAIN":       0,
				"ya.ru._response_per_rcode_NXRRSET":        0,
				"ya.ru._response_per_rcode_REFUSED":        0,
				"ya.ru._response_per_rcode_SERVFAIL":       21,
				"ya.ru._response_per_rcode_YXDOMAIN":       0,
				"ya.ru._response_per_rcode_YXRRSET":        0,
				"ya.ru._response_per_rcode_other":          0,
				"ya.ru._response_total":                    21,
			}

			assert.Equal(t, expected, collr.Collect(context.Background()))
		})
	}
}

func TestCollector_CollectNoLoad(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"version 1.6.9", dataVer169NoLoad},
		{"version 1.7.0", dataVer170NoLoad},
	}
	for _, testNoLoad := range tests {
		t.Run(testNoLoad.name, func(t *testing.T) {
			ts := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write(testNoLoad.data)
					}))
			defer ts.Close()

			collr := New()
			collr.URL = ts.URL + "/metrics"
			collr.PerServerStats.Includes = []string{"glob:*"}
			collr.PerZoneStats.Includes = []string{"glob:*"}
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))

			expected := map[string]int64{
				"no_matching_zone_dropped_total": 0,
				"panic_total":                    99,
				"request_per_ip_family_v4":       0,
				"request_per_ip_family_v6":       0,
				"request_per_proto_tcp":          0,
				"request_per_proto_udp":          0,
				"request_per_status_dropped":     0,
				"request_per_status_processed":   0,
				"request_per_type_A":             0,
				"request_per_type_AAAA":          0,
				"request_per_type_ANY":           0,
				"request_per_type_CNAME":         0,
				"request_per_type_DNSKEY":        0,
				"request_per_type_DS":            0,
				"request_per_type_IXFR":          0,
				"request_per_type_MX":            0,
				"request_per_type_NS":            0,
				"request_per_type_NSEC":          0,
				"request_per_type_NSEC3":         0,
				"request_per_type_PTR":           0,
				"request_per_type_RRSIG":         0,
				"request_per_type_SOA":           0,
				"request_per_type_SRV":           0,
				"request_per_type_TXT":           0,
				"request_per_type_other":         0,
				"request_total":                  0,
				"response_per_rcode_BADALG":      0,
				"response_per_rcode_BADCOOKIE":   0,
				"response_per_rcode_BADKEY":      0,
				"response_per_rcode_BADMODE":     0,
				"response_per_rcode_BADNAME":     0,
				"response_per_rcode_BADSIG":      0,
				"response_per_rcode_BADTIME":     0,
				"response_per_rcode_BADTRUNC":    0,
				"response_per_rcode_FORMERR":     0,
				"response_per_rcode_NOERROR":     0,
				"response_per_rcode_NOTAUTH":     0,
				"response_per_rcode_NOTIMP":      0,
				"response_per_rcode_NOTZONE":     0,
				"response_per_rcode_NXDOMAIN":    0,
				"response_per_rcode_NXRRSET":     0,
				"response_per_rcode_REFUSED":     0,
				"response_per_rcode_SERVFAIL":    0,
				"response_per_rcode_YXDOMAIN":    0,
				"response_per_rcode_YXRRSET":     0,
				"response_per_rcode_other":       0,
				"response_total":                 0,
			}

			assert.Equal(t, expected, collr.Collect(context.Background()))
		})
	}

}

func TestCollector_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_CollectNoVersion(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataNoLoadNoVersion)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/metrics"
	collr.PerServerStats.Includes = []string{"glob:*"}
	collr.PerZoneStats.Includes = []string{"glob:*"}
	require.NoError(t, collr.Init(context.Background()))
	require.Error(t, collr.Check(context.Background()))

	assert.Nil(t, collr.Collect(context.Background()))
}
