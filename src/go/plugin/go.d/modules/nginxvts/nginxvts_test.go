// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer0118Response, _ = os.ReadFile("testdata/vts-v0.1.18.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataVer0118Response": dataVer0118Response,
	} {
		require.NotNil(t, data, name)
	}
}

func TestNginxVTS_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &NginxVTS{}, dataConfigJSON, dataConfigYAML)
}

func TestNginxVTS_Init(t *testing.T) {
	tests := map[string]struct {
		config          Config
		wantNumOfCharts int
		wantFail        bool
	}{
		"default": {
			wantNumOfCharts: numOfCharts(
				mainCharts,
				sharedZonesCharts,
				serverZonesCharts,
			),
			config: New().Config,
		},
		"URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				}},
		},
		"invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					ClientConfig: web.ClientConfig{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			es := New()
			es.Config = test.config

			if test.wantFail {
				assert.Error(t, es.Init())
			} else {
				assert.NoError(t, es.Init())
				assert.Equal(t, test.wantNumOfCharts, len(*es.Charts()))
			}
		})
	}
}

func TestNginxVTS_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (vts *NginxVTS, cleanup func())
		wantFail bool
	}{
		"valid data":         {prepare: prepareNginxVTSValidData},
		"invalid data":       {prepare: prepareNginxVTSInvalidData, wantFail: true},
		"404":                {prepare: prepareNginxVTS404, wantFail: true},
		"connection refused": {prepare: prepareNginxVTSConnectionRefused, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			vts, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, vts.Check())
			} else {
				assert.NoError(t, vts.Check())
			}
		})
	}
}

func TestNginxVTS_Charts(t *testing.T) {
	assert.Nil(t, New().Charts())
}

func TestNginxVTS_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestNginxVTS_Collect(t *testing.T) {
	tests := map[string]struct {
		// prepare       func() *NginxVTS
		prepare       func(t *testing.T) (vts *NginxVTS, cleanup func())
		wantCollected map[string]int64
		checkCharts   bool
	}{
		"right metrics": {
			prepare: prepareNginxVTSValidData,
			wantCollected: map[string]int64{
				// Nginx running time
				"uptime": 319,
				// Nginx connections
				"connections_active":   2,
				"connections_reading":  0,
				"connections_writing":  1,
				"connections_waiting":  1,
				"connections_accepted": 12,
				"connections_handled":  12,
				"connections_requests": 17,
				// Nginx shared memory
				"sharedzones_maxsize":  1048575,
				"sharedzones_usedsize": 45799,
				"sharedzones_usednode": 13,
				// Nginx traffic
				"total_requestcounter": 2,
				"total_inbytes":        156,
				"total_outbytes":       692,
				// Nginx response code
				"total_responses_1xx": 1,
				"total_responses_2xx": 2,
				"total_responses_3xx": 3,
				"total_responses_4xx": 4,
				"total_responses_5xx": 5,
				// Nginx cache
				"total_cache_miss":        2,
				"total_cache_bypass":      4,
				"total_cache_expired":     6,
				"total_cache_stale":       8,
				"total_cache_updating":    10,
				"total_cache_revalidated": 12,
				"total_cache_hit":         14,
				"total_cache_scarce":      16,
			},
			checkCharts: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			vts, cleanup := test.prepare(t)
			defer cleanup()

			collected := vts.Collect()

			assert.Equal(t, test.wantCollected, collected)
			if test.checkCharts {
				ensureCollectedHasAllChartsDimsVarsIDs(t, vts, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, vts *NginxVTS, collected map[string]int64) {
	for _, chart := range *vts.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareNginxVTS(t *testing.T, createNginxVTS func() *NginxVTS) (vts *NginxVTS, cleanup func()) {
	t.Helper()
	vts = createNginxVTS()
	srv := prepareNginxVTSEndpoint()
	vts.URL = srv.URL

	require.NoError(t, vts.Init())

	return vts, srv.Close
}

func prepareNginxVTSValidData(t *testing.T) (vts *NginxVTS, cleanup func()) {
	return prepareNginxVTS(t, New)
}

func prepareNginxVTSInvalidData(t *testing.T) (*NginxVTS, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	vts := New()
	vts.URL = srv.URL
	require.NoError(t, vts.Init())

	return vts, srv.Close
}

func prepareNginxVTS404(t *testing.T) (*NginxVTS, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	vts := New()
	vts.URL = srv.URL
	require.NoError(t, vts.Init())

	return vts, srv.Close
}

func prepareNginxVTSConnectionRefused(t *testing.T) (*NginxVTS, func()) {
	t.Helper()
	vts := New()
	vts.URL = "http://127.0.0.1:18080"
	require.NoError(t, vts.Init())

	return vts, func() {}
}

func prepareNginxVTSEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				_, _ = w.Write(dataVer0118Response)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}

func numOfCharts(charts ...module.Charts) (num int) {
	for _, v := range charts {
		num += len(v)
	}
	return num
}
