// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

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

	dataStatusJSON, _           = os.ReadFile("testdata/status.json")
	dataStatusFullJSON, _       = os.ReadFile("testdata/status-full.json")
	dataStatusFullNoIdleJSON, _ = os.ReadFile("testdata/status-full-no-idle.json")
	dataStatusText, _           = os.ReadFile("testdata/status.txt")
	dataStatusFullText, _       = os.ReadFile("testdata/status-full.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataStatusJSON":           dataStatusJSON,
		"dataStatusFullJSON":       dataStatusFullJSON,
		"dataStatusFullNoIdleJSON": dataStatusFullNoIdleJSON,
		"dataStatusText":           dataStatusText,
		"dataStatusFullText":       dataStatusFullText,
	} {
		require.NotNil(t, data, name)
	}
}

func TestPhpfpm_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Phpfpm{}, dataConfigJSON, dataConfigYAML)
}

func TestPhpfpm_Init(t *testing.T) {
	job := New()

	require.NoError(t, job.Init())
	assert.NotNil(t, job.client)
}

func TestPhpfpm_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.NoError(t, job.Check())
}

func TestPhpfpm_CheckReturnsFalseOnFailure(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/us"
	require.NoError(t, job.Init())

	assert.Error(t, job.Check())
}

func TestPhpfpm_Charts(t *testing.T) {
	job := New()

	assert.NotNil(t, job.Charts())
}

func TestPhpfpm_CollectJSON(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.NoError(t, job.Init())

	got := job.Collect()

	want := map[string]int64{
		"active":    1,
		"idle":      1,
		"maxActive": 1,
		"reached":   0,
		"requests":  21,
		"slow":      0,
	}
	assert.Equal(t, want, got)
}

func TestPhpfpm_CollectJSONFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.NoError(t, job.Init())

	got := job.Collect()

	want := map[string]int64{
		"active":    1,
		"idle":      1,
		"maxActive": 1,
		"reached":   0,
		"requests":  22,
		"slow":      0,
		"minReqCpu": 0,
		"maxReqCpu": 10,
		"avgReqCpu": 5,
		"minReqDur": 0,
		"maxReqDur": 919,
		"avgReqDur": 459,
		"minReqMem": 2093045,
		"maxReqMem": 2097152,
		"avgReqMem": 2095098,
	}
	assert.Equal(t, want, got)
}

func TestPhpfpm_CollectNoIdleProcessesJSONFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullNoIdleJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.NoError(t, job.Init())

	got := job.Collect()

	want := map[string]int64{
		"active":    1,
		"idle":      1,
		"maxActive": 1,
		"reached":   0,
		"requests":  22,
		"slow":      0,
	}
	assert.Equal(t, want, got)
}

func TestPhpfpm_CollectText(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	got := job.Collect()

	want := map[string]int64{
		"active":    1,
		"idle":      1,
		"maxActive": 1,
		"reached":   0,
		"requests":  19,
		"slow":      0,
	}
	assert.Equal(t, want, got)
}

func TestPhpfpm_CollectTextFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	got := job.Collect()

	want := map[string]int64{
		"active":    1,
		"idle":      1,
		"maxActive": 1,
		"reached":   0,
		"requests":  20,
		"slow":      0,
		"minReqCpu": 0,
		"maxReqCpu": 10,
		"avgReqCpu": 5,
		"minReqDur": 0,
		"maxReqDur": 536,
		"avgReqDur": 268,
		"minReqMem": 2093045,
		"maxReqMem": 2097152,
		"avgReqMem": 2095098,
	}
	assert.Equal(t, want, got)
}

func TestPhpfpm_CollectReturnsNothingWhenInvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye\nfrom someone\nfoobar"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Len(t, job.Collect(), 0)
}

func TestPhpfpm_CollectReturnsNothingWhenEmptyData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte{})
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Len(t, job.Collect(), 0)
}

func TestPhpfpm_CollectReturnsNothingWhenBadStatusCode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.NoError(t, job.Init())

	assert.Len(t, job.Collect(), 0)
}

func TestPhpfpm_Cleanup(t *testing.T) {
	New().Cleanup()
}
