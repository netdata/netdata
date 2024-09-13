// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

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

	dataStatusMetrics, _        = os.ReadFile("testdata/status.txt")
	dataTengineStatusMetrics, _ = os.ReadFile("testdata/tengine-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataStatusMetrics":        dataStatusMetrics,
		"dataTengineStatusMetrics": dataTengineStatusMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestNginx_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Nginx{}, dataConfigJSON, dataConfigYAML)
}

func TestNginx_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestNginx_Init(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
}

func TestNginx_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
}

func TestNginx_CheckNG(t *testing.T) {
	job := New()

	job.URL = "http://127.0.0.1:38001/us"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestNginx_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestNginx_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"accepts":  36,
		"active":   1,
		"handled":  36,
		"reading":  0,
		"requests": 126,
		"waiting":  0,
		"writing":  1,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestNginx_CollectTengine(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataTengineStatusMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"accepts":      1140,
		"active":       1,
		"handled":      1140,
		"reading":      0,
		"request_time": 75806,
		"requests":     1140,
		"waiting":      0,
		"writing":      1,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestNginx_InvalidData(t *testing.T) {
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

func TestNginx_404(t *testing.T) {
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
