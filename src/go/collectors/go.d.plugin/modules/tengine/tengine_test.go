// SPDX-License-Identifier: GPL-3.0-or-later

package tengine

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
	testStatusData, _ = os.ReadFile("testdata/status.txt")
)

func TestTengine_Cleanup(t *testing.T) { New().Cleanup() }

func TestNew(t *testing.T) {
	job := New()

	assert.Implements(t, (*module.Module)(nil), job)
	assert.Equal(t, defaultURL, job.URL)
	assert.Equal(t, defaultHTTPTimeout, job.Timeout.Duration)
}

func TestTengine_Init(t *testing.T) {
	job := New()

	require.True(t, job.Init())
	assert.NotNil(t, job.apiClient)
}

func TestTengine_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testStatusData)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	assert.True(t, job.Check())
}

func TestTengine_CheckNG(t *testing.T) {
	job := New()

	job.URL = "http://127.0.0.1:38001/us"
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestTengine_Charts(t *testing.T) { assert.NotNil(t, New().Charts()) }

func TestTengine_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testStatusData)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	require.True(t, job.Check())

	expected := map[string]int64{
		"bytes_in":                 5944,
		"bytes_out":                20483,
		"conn_total":               354,
		"http_200":                 1536,
		"http_206":                 0,
		"http_2xx":                 1536,
		"http_302":                 43,
		"http_304":                 0,
		"http_3xx":                 50,
		"http_403":                 1,
		"http_404":                 75,
		"http_416":                 0,
		"http_499":                 0,
		"http_4xx":                 80,
		"http_500":                 0,
		"http_502":                 1,
		"http_503":                 0,
		"http_504":                 0,
		"http_508":                 0,
		"http_5xx":                 1,
		"http_other_detail_status": 11,
		"http_other_status":        0,
		"http_ups_4xx":             26,
		"http_ups_5xx":             1,
		"req_total":                1672,
		"rt":                       1339,
		"ups_req":                  268,
		"ups_rt":                   644,
		"ups_tries":                268,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestTengine_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestTengine_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	assert.False(t, job.Check())
}
