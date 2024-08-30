// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

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

	dataMetrics, _ = os.ReadFile("testdata/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataMetrics":    dataMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestKubeProxy_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &KubeProxy{}, dataConfigJSON, dataConfigYAML)
}

func TestKubeProxy_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestKubeProxy_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestKubeProxy_Init(t *testing.T) {
	assert.NoError(t, New().Init())
}

func TestKubeProxy_InitNG(t *testing.T) {
	job := New()
	job.URL = ""
	assert.Error(t, job.Init())
}

func TestKubeProxy_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
}

func TestKubeProxy_CheckNG(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/metrics"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestKubeProxy_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataMetrics)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	expected := map[string]int64{
		"sync_proxy_rules_count":           2669,
		"sync_proxy_rules_bucket_1000":     1,
		"sync_proxy_rules_bucket_2000":     0,
		"sync_proxy_rules_bucket_4000":     0,
		"sync_proxy_rules_bucket_8000":     0,
		"sync_proxy_rules_bucket_16000":    23,
		"sync_proxy_rules_bucket_32000":    2510,
		"sync_proxy_rules_bucket_64000":    126,
		"sync_proxy_rules_bucket_128000":   8,
		"sync_proxy_rules_bucket_256000":   0,
		"sync_proxy_rules_bucket_512000":   1,
		"sync_proxy_rules_bucket_1024000":  0,
		"sync_proxy_rules_bucket_4096000":  0,
		"sync_proxy_rules_bucket_8192000":  0,
		"sync_proxy_rules_bucket_2048000":  0,
		"sync_proxy_rules_bucket_16384000": 0,
		"sync_proxy_rules_bucket_+Inf":     0,
		"rest_client_requests_201":         1,
		"rest_client_requests_200":         362,
		"rest_client_requests_GET":         362,
		"rest_client_requests_POST":        1,
		"http_request_duration_05":         1515,
		"http_request_duration_09":         3939,
		"http_request_duration_099":        9464,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestKubeProxy_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestKubeProxy_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}
