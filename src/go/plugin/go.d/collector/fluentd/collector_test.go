// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	// OK
	collr := New()
	assert.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.apiClient)

	//NG
	collr = New()
	collr.URL = ""
	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataPluginsMetrics)
		}))
	defer ts.Close()

	// OK
	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	// NG
	collr = New()
	collr.URL = "http://127.0.0.1:38001/api/plugins.json"
	require.NoError(t, collr.Init(context.Background()))
	require.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Collect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataPluginsMetrics)
		}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"output_stdout_stdout_output_retry_count":         0,
		"output_td_tdlog_output_retry_count":              0,
		"output_td_tdlog_output_buffer_queue_length":      0,
		"output_td_tdlog_output_buffer_total_queued_size": 0,
	}
	assert.Equal(t, expected, collr.Collect(context.Background()))
	assert.Len(t, collr.charts.Get("retry_count").Dims, 2)
	assert.Len(t, collr.charts.Get("buffer_queue_length").Dims, 1)
	assert.Len(t, collr.charts.Get("buffer_total_queued_size").Dims, 1)
}

func TestCollector_InvalidData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and goodbye"))
		}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
