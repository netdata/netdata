// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	repo1Data, _ = os.ReadFile("testdata/repo1.txt")
	repo2Data, _ = os.ReadFile("testdata/repo2.txt")
	repo3Data, _ = os.ReadFile("testdata/repo3.txt")
)

func TestNew(t *testing.T) {
	job := New()

	assert.IsType(t, (*DockerHub)(nil), job)
	assert.Equal(t, defaultURL, job.URL)
	assert.Equal(t, defaultHTTPTimeout, job.Timeout.Duration)
	assert.Len(t, job.Repositories, 0)
	assert.Nil(t, job.client)
}

func TestDockerHub_Charts(t *testing.T) { assert.NotNil(t, New().Charts()) }

func TestDockerHub_Cleanup(t *testing.T) { New().Cleanup() }

func TestDockerHub_Init(t *testing.T) {
	job := New()
	job.Repositories = []string{"name/repo"}
	assert.True(t, job.Init())
	assert.NotNil(t, job.client)
}

func TestDockerHub_InitNG(t *testing.T) { assert.False(t, New().Init()) }

func TestDockerHub_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "name1/repo1"):
					_, _ = w.Write(repo1Data)
				case strings.HasSuffix(r.URL.Path, "name2/repo2"):
					_, _ = w.Write(repo2Data)
				case strings.HasSuffix(r.URL.Path, "name3/repo3"):
					_, _ = w.Write(repo3Data)
				}
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.True(t, job.Init())
	assert.True(t, job.Check())
}

func TestDockerHub_CheckNG(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/metrics"
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestDockerHub_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "name1/repo1"):
					_, _ = w.Write(repo1Data)
				case strings.HasSuffix(r.URL.Path, "name2/repo2"):
					_, _ = w.Write(repo2Data)
				case strings.HasSuffix(r.URL.Path, "name3/repo3"):
					_, _ = w.Write(repo3Data)
				}
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.True(t, job.Init())
	require.True(t, job.Check())

	expected := map[string]int64{
		"star_count_user1/name1": 45,
		"pull_count_user1/name1": 18540191,
		"status_user1/name1":     1,
		"star_count_user2/name2": 45,
		"pull_count_user2/name2": 18540192,
		"status_user2/name2":     1,
		"star_count_user3/name3": 45,
		"pull_count_user3/name3": 18540193,
		"status_user3/name3":     1,
		"pull_sum":               55620576,
	}

	collected := job.Collect()

	for k := range collected {
		if strings.HasPrefix(k, "last") {
			delete(collected, k)
		}
	}
	assert.Equal(t, expected, collected)
}

func TestDockerHub_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestDockerHub_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.True(t, job.Init())
	assert.False(t, job.Check())
}
