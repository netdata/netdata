// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
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

	dataStatusMetrics, _       = os.ReadFile("testdata/status.txt")
	dataApacheStatusMetrics, _ = os.ReadFile("testdata/apache-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestLighttpd_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Lighttpd{}, dataConfigJSON, dataConfigYAML)
}

func TestLighttpd_Cleanup(t *testing.T) { New().Cleanup() }

func TestLighttpd_Init(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
}

func TestLighttpd_InitNG(t *testing.T) {
	job := New()

	job.URL = ""
	assert.Error(t, job.Init())
}

func TestLighttpd_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/server-status?auto"
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
}

func TestLighttpd_CheckNG(t *testing.T) {
	job := New()

	job.URL = "http://127.0.0.1:38001/server-status?auto"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestLighttpd_Charts(t *testing.T) { assert.NotNil(t, New().Charts()) }

func TestLighttpd_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/server-status?auto"
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"scoreboard_waiting":        125,
		"scoreboard_request_end":    0,
		"busy_servers":              3,
		"scoreboard_keepalive":      1,
		"scoreboard_read":           1,
		"scoreboard_request_start":  0,
		"scoreboard_response_start": 0,
		"scoreboard_close":          0,
		"scoreboard_open":           0,
		"scoreboard_hard_error":     0,
		"scoreboard_handle_request": 1,
		"idle_servers":              125,
		"total_kBytes":              4,
		"uptime":                    11,
		"scoreboard_read_post":      0,
		"scoreboard_write":          0,
		"scoreboard_response_end":   0,
		"total_accesses":            12,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestLighttpd_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/server-status?auto"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestLighttpd_ApacheData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataApacheStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/server-status?auto"
	require.NoError(t, job.Init())
	require.Error(t, job.Check())
}

func TestLighttpd_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/server-status?auto"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}
