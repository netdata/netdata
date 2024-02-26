// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testStatusJSON, _           = os.ReadFile("testdata/status.json")
	testStatusFullJSON, _       = os.ReadFile("testdata/status-full.json")
	testStatusFullNoIdleJSON, _ = os.ReadFile("testdata/status-full-no-idle.json")
	testStatusText, _           = os.ReadFile("testdata/status.txt")
	testStatusFullText, _       = os.ReadFile("testdata/status-full.txt")
)

func Test_readTestData(t *testing.T) {
	assert.NotNil(t, testStatusJSON)
	assert.NotNil(t, testStatusFullJSON)
	assert.NotNil(t, testStatusFullNoIdleJSON)
	assert.NotNil(t, testStatusText)
	assert.NotNil(t, testStatusFullText)
}

func TestNew(t *testing.T) {
	job := New()

	assert.Implements(t, (*module.Module)(nil), job)
}

func TestPhpfpm_Init(t *testing.T) {
	job := New()

	got := job.Init()

	require.True(t, got)
	assert.NotNil(t, job.client)
}

func TestPhpfpm_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testStatusText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Init()
	require.True(t, job.Init())

	got := job.Check()

	assert.True(t, got)
}

func TestPhpfpm_CheckReturnsFalseOnFailure(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/us"
	require.True(t, job.Init())

	got := job.Check()

	assert.False(t, got)
}

func TestPhpfpm_Charts(t *testing.T) {
	job := New()

	got := job.Charts()

	assert.NotNil(t, got)
}

func TestPhpfpm_CollectJSON(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testStatusJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.True(t, job.Init())

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
				_, _ = w.Write(testStatusFullJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.True(t, job.Init())

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
				_, _ = w.Write(testStatusFullNoIdleJSON)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/?json"
	require.True(t, job.Init())

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
				_, _ = w.Write(testStatusText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())

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
				_, _ = w.Write(testStatusFullText)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())

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
	require.True(t, job.Init())

	got := job.Collect()

	assert.Len(t, got, 0)
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
	require.True(t, job.Init())

	got := job.Collect()

	assert.Len(t, got, 0)
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
	require.True(t, job.Init())

	got := job.Collect()

	assert.Len(t, got, 0)
}

func TestPhpfpm_Cleanup(t *testing.T) {
	New().Cleanup()
}
