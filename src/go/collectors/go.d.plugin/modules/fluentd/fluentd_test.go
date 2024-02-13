// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDataPlugins, _ = os.ReadFile("testdata/plugins.json")

func TestNew(t *testing.T) {
	job := New()
	assert.IsType(t, (*Fluentd)(nil), job)
	assert.NotNil(t, job.charts)
	assert.NotNil(t, job.activePlugins)
	assert.Equal(t, defaultURL, job.URL)
	assert.Equal(t, defaultHTTPTimeout, job.Timeout.Duration)
}

func TestFluentd_Init(t *testing.T) {
	// OK
	job := New()
	assert.True(t, job.Init())
	assert.NotNil(t, job.apiClient)

	//NG
	job = New()
	job.URL = ""
	assert.False(t, job.Init())
}

func TestFluentd_Check(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(testDataPlugins)
		}))
	defer ts.Close()

	// OK
	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	require.True(t, job.Check())

	// NG
	job = New()
	job.URL = "http://127.0.0.1:38001/api/plugins.json"
	require.True(t, job.Init())
	require.False(t, job.Check())
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
			_, _ = w.Write(testDataPlugins)
		}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL

	require.True(t, job.Init())
	require.True(t, job.Check())

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
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestFluentd_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	require.True(t, job.Init())
	assert.False(t, job.Check())
}
