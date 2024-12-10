// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) { New().Cleanup(context.Background()) }

func TestCollector_Init(t *testing.T) {
	collr := New()

	require.NoError(t, collr.Init(context.Background()))
}

func TestCollector_InitNG(t *testing.T) {
	collr := New()

	collr.URL = ""
	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckNG(t *testing.T) {
	collr := New()

	collr.URL = "http://127.0.0.1:38001/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) { assert.NotNil(t, New().Charts()) }

func TestCollector_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

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

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_ApacheData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataApacheStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	require.Error(t, collr.Check(context.Background()))
}

func TestCollector_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
