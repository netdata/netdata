// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

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

	dataFullStatusMetrics, _ = os.ReadFile("testdata/fullstatus.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":        dataConfigJSON,
		"dataConfigYAML":        dataConfigYAML,
		"dataFullStatusMetrics": dataFullStatusMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	require.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckNG(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	collr := New()

	assert.NotNil(t, collr.Charts())
	assert.False(t, collr.charts.Has(uptimeChart.ID))

	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
	assert.True(t, collr.charts.Has(uptimeChart.ID))
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"alive":       350,
		"busy":        200,
		"idle":        50,
		"init":        20,
		"initialized": 10,
		"preinit":     20,
		"reloading":   100,
		"shutdown":    500,
		"uptime":      15765,
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
	collr.URL = ts.URL
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
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
