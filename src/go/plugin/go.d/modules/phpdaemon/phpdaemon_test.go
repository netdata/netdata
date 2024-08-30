// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

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

func TestPHPDaemon_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &PHPDaemon{}, dataConfigJSON, dataConfigYAML)
}

func TestPHPDaemon_Init(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
	assert.NotNil(t, job.client)
}

func TestPHPDaemon_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
}

func TestPHPDaemon_CheckNG(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestPHPDaemon_Charts(t *testing.T) {
	job := New()

	assert.NotNil(t, job.Charts())
	assert.False(t, job.charts.Has(uptimeChart.ID))

	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
	assert.True(t, job.charts.Has(uptimeChart.ID))
}

func TestPHPDaemon_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestPHPDaemon_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataFullStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())

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

	assert.Equal(t, expected, job.Collect())

}

func TestPHPDaemon_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestPHPDaemon_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}
