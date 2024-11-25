// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataRepo1, _ = os.ReadFile("testdata/repo1.txt")
	dataRepo2, _ = os.ReadFile("testdata/repo2.txt")
	dataRepo3, _ = os.ReadFile("testdata/repo3.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataRepo1":      dataRepo1,
		"dataRepo2":      dataRepo2,
		"dataRepo3":      dataRepo3,
	} {
		require.NotNil(t, data, name)
	}
}

func TestDockerHub_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &DockerHub{}, dataConfigJSON, dataConfigYAML)
}

func TestDockerHub_Charts(t *testing.T) { assert.NotNil(t, New().Charts()) }

func TestDockerHub_Cleanup(t *testing.T) { New().Cleanup() }

func TestDockerHub_Init(t *testing.T) {
	job := New()
	job.Repositories = []string{"name/repo"}
	assert.NoError(t, job.Init())
	assert.NotNil(t, job.client)
}

func TestDockerHub_InitNG(t *testing.T) {
	assert.Error(t, New().Init())
}

func TestDockerHub_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "name1/repo1"):
					_, _ = w.Write(dataRepo1)
				case strings.HasSuffix(r.URL.Path, "name2/repo2"):
					_, _ = w.Write(dataRepo2)
				case strings.HasSuffix(r.URL.Path, "name3/repo3"):
					_, _ = w.Write(dataRepo3)
				}
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.NoError(t, job.Init())
	assert.NoError(t, job.Check())
}

func TestDockerHub_CheckNG(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/metrics"
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}

func TestDockerHub_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "name1/repo1"):
					_, _ = w.Write(dataRepo1)
				case strings.HasSuffix(r.URL.Path, "name2/repo2"):
					_, _ = w.Write(dataRepo2)
				case strings.HasSuffix(r.URL.Path, "name3/repo3"):
					_, _ = w.Write(dataRepo3)
				}
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL
	job.Repositories = []string{"name1/repo1", "name2/repo2", "name3/repo3"}
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

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
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
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
	require.NoError(t, job.Init())
	assert.Error(t, job.Check())
}
