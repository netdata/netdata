// SPDX-License-Identifier: GPL-3.0-or-later

package springboot2

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testdata, _  = os.ReadFile("tests/testdata.txt")
	testdata2, _ = os.ReadFile("tests/testdata2.txt")
)

func TestSpringboot2_Collect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/actuator/prometheus":
			_, _ = w.Write(testdata)
		case "/actuator/prometheus2":
			_, _ = w.Write(testdata2)
		}
	}))
	defer ts.Close()
	job1 := New()
	job1.HTTP.Request.URL = ts.URL + "/actuator/prometheus"
	assert.True(t, job1.Init())
	assert.True(t, job1.Check())
	assert.EqualValues(
		t,
		map[string]int64{
			"threads":                 23,
			"threads_daemon":          21,
			"resp_1xx":                1,
			"resp_2xx":                19,
			"resp_3xx":                1,
			"resp_4xx":                4,
			"resp_5xx":                1,
			"heap_used_eden":          129649936,
			"heap_used_survivor":      8900136,
			"heap_used_old":           17827920,
			"heap_committed_eden":     153616384,
			"heap_committed_survivor": 8912896,
			"heap_committed_old":      40894464,
			"mem_free":                47045752,
			"uptime":                  191730,
		},
		job1.Collect(),
	)

	job2 := New()
	job2.HTTP.Request.URL = ts.URL + "/actuator/prometheus2"
	assert.True(t, job2.Init())
	assert.True(t, job2.Check())
	assert.EqualValues(
		t,
		map[string]int64{
			"threads":                 36,
			"threads_daemon":          22,
			"resp_1xx":                0,
			"resp_2xx":                57740,
			"resp_3xx":                0,
			"resp_4xx":                4,
			"resp_5xx":                0,
			"heap_used_eden":          18052960,
			"heap_used_survivor":      302704,
			"heap_used_old":           40122672,
			"heap_committed_eden":     21430272,
			"heap_committed_survivor": 2621440,
			"heap_committed_old":      53182464,
			"mem_free":                18755840,
			"uptime":                  45501125,
		},
		job2.Collect(),
	)
}

func TestSpringboot2_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()
	job := New()
	job.HTTP.Request.URL = ts.URL + "/actuator/prometheus"

	job.Init()

	assert.False(t, job.Check())

	job.Cleanup()
}

func TestSpringBoot2_Charts(t *testing.T) {
	job := New()
	charts := job.Charts()

	assert.True(t, charts.Has("response_codes"))
	assert.True(t, charts.Has("uptime"))
}
