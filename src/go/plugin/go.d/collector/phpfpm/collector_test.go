// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.client)
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusText)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckReturnsFalseOnFailure(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001/us"
	require.NoError(t, collr.Init(context.Background()))

	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	collr := New()

	assert.NotNil(t, collr.Charts())
}

func TestCollector_CollectJSON(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusJSON)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/?json"
	require.NoError(t, collr.Init(context.Background()))

	got := collr.Collect(context.Background())

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

func TestCollector_CollectJSONFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullJSON)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/?json"
	require.NoError(t, collr.Init(context.Background()))

	got := collr.Collect(context.Background())

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

func TestCollector_CollectNoIdleProcessesJSONFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullNoIdleJSON)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/?json"
	require.NoError(t, collr.Init(context.Background()))

	got := collr.Collect(context.Background())

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

func TestCollector_CollectText(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusText)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	got := collr.Collect(context.Background())

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

func TestCollector_CollectTextFull(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusFullText)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	got := collr.Collect(context.Background())

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

func TestCollector_CollectReturnsNothingWhenInvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye\nfrom someone\nfoobar"))
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	assert.Len(t, collr.Collect(context.Background()), 0)
}

func TestCollector_CollectReturnsNothingWhenEmptyData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte{})
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	assert.Len(t, collr.Collect(context.Background()), 0)
}

func TestCollector_CollectReturnsNothingWhenBadStatusCode(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))

	assert.Len(t, collr.Collect(context.Background()), 0)
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}
