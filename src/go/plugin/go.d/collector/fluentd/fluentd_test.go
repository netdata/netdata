// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

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

	dataPluginsMetrics, _ = os.ReadFile("testdata/plugins.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataPluginsMetrics": dataPluginsMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestFluentd_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Fluentd{}, dataConfigJSON, dataConfigYAML)
}

func TestFluentd_Init(t *testing.T) {
	// OK
	job := New()
	assert.NoError(t, job.Init())
	assert.NotNil(t, job.apiClient)

	//NG
	job = New()
	job.URL = ""
	assert.Error(t, job.Init())
}

func TestFluentd_Check(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataPluginsMetrics)
		}))
	defer ts.Close()

	// OK
	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	// NG
	job = New()
	job.URL = "http://127.0.0.1:38001/api/plugins.json"
	require.NoError(t, job.Init())
	require.Error(t, job.Check())
}

func TestFluentd_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestFluentd_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestFluentd_Collect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataPluginsMetrics)
		}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL

	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"output_stdout_stdout_output_retry_count":         0,
		"output_td_tdlog_output_retry_count":              0,
		"output_td_tdlog_output_buffer_queue_length":      0,
		"output_td_tdlog_output_buffer_total_queued_size": 0,
	}
	assert.Equal(t, expected, job.Collect())
	assert.Len(t, job.charts.Get("retry_count").Dims, 2)
	assert.Len(t, job.charts.Get("buffer_queue_length").Dims, 1)
	assert.Len(t, job.charts.Get("buffer_total_queued_size").Dims, 1)
}

func TestFluentd_InvalidData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and goodbye"))
		}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestFluentd_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}
