// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

var (
	testData, _       = os.ReadFile("testdata/testdata.txt")
	testDataNoMeta, _ = os.ReadFile("testdata/testdata.nometa.txt")
)

func Test_testClientDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"testData": testData,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestPrometheus404(t *testing.T) {
	tsMux := http.NewServeMux()
	tsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	ts := httptest.NewServer(tsMux)
	defer ts.Close()

	req := web.RequestConfig{URL: ts.URL + "/metrics"}
	prom := New(http.DefaultClient, req)
	res, err := prom.ScrapeSeries()

	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestPrometheusPlain(t *testing.T) {
	tsMux := http.NewServeMux()
	tsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testData)
	})
	ts := httptest.NewServer(tsMux)
	defer ts.Close()

	req := web.RequestConfig{URL: ts.URL + "/metrics"}
	prom := New(http.DefaultClient, req)
	res, err := prom.ScrapeSeries()

	assert.NoError(t, err)
	verifyTestData(t, res)
}

func TestPrometheusScrapeModel(t *testing.T) {
	tests := map[string]struct {
		selectorExpr string
		verify       func(t *testing.T, samples promscrapemodel.Samples)
	}{
		"plain": {
			verify: func(t *testing.T, samples promscrapemodel.Samples) {
				require.NotEmpty(t, samples)

				sample, ok := findScrapeModelSample(samples, "go_gc_duration_seconds", map[string]string{"quantile": "0.25"})
				require.True(t, ok)
				assert.Equal(t, "0.25", sample.Labels.Get("quantile"))
				assert.Equal(t, model.MetricTypeSummary, sample.FamilyType)
			},
		},
		"with selector": {
			selectorExpr: "go_gc*",
			verify: func(t *testing.T, samples promscrapemodel.Samples) {
				require.NotEmpty(t, samples)
				for _, sample := range samples {
					assert.Truef(t, strings.HasPrefix(sample.Name, "go_gc"), sample.Name)
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tsMux := http.NewServeMux()
			tsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testData)
			})
			ts := httptest.NewServer(tsMux)
			defer ts.Close()

			req := web.RequestConfig{URL: ts.URL + "/metrics"}

			var prom Prometheus
			if test.selectorExpr != "" {
				sr, err := selector.Parse(test.selectorExpr)
				require.NoError(t, err)
				prom = NewWithSelector(http.DefaultClient, req, sr)
			} else {
				prom = New(http.DefaultClient, req)
			}

			res, err := prom.Scrape()
			require.NoError(t, err)
			test.verify(t, res)
		})
	}
}

func TestPrometheusPlainWithSelector(t *testing.T) {
	tsMux := http.NewServeMux()
	tsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testData)
	})
	ts := httptest.NewServer(tsMux)
	defer ts.Close()

	req := web.RequestConfig{URL: ts.URL + "/metrics"}
	sr, err := selector.Parse("go_gc*")
	require.NoError(t, err)
	prom := NewWithSelector(http.DefaultClient, req, sr)

	res, err := prom.ScrapeSeries()
	require.NoError(t, err)

	for _, v := range res {
		assert.Truef(t, strings.HasPrefix(v.Name(), "go_gc"), v.Name())
	}
}

func TestPrometheusGzip(t *testing.T) {
	counter := 0
	rawTestData := [][]byte{testData, testDataNoMeta}
	tsMux := http.NewServeMux()
	tsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(200)
		gz := new(bytes.Buffer)
		ww := gzip.NewWriter(gz)
		_, _ = ww.Write(rawTestData[counter])
		_ = ww.Close()
		_, _ = gz.WriteTo(w)
		counter++
	})
	ts := httptest.NewServer(tsMux)
	defer ts.Close()

	req := web.RequestConfig{URL: ts.URL + "/metrics"}
	prom := New(http.DefaultClient, req)

	for i := 0; i < 2; i++ {
		res, err := prom.ScrapeSeries()
		assert.NoError(t, err)
		verifyTestData(t, res)
	}
}

func TestPrometheusReadFromFile(t *testing.T) {
	req := web.RequestConfig{URL: "file://testdata/testdata.txt"}

	prom := NewWithSelector(http.DefaultClient, req, nil)

	for i := 0; i < 2; i++ {
		res, err := prom.ScrapeSeries()
		assert.NoError(t, err)
		verifyTestData(t, res)
	}

	prom = New(http.DefaultClient, req)

	for i := 0; i < 2; i++ {
		res, err := prom.ScrapeSeries()
		assert.NoError(t, err)
		verifyTestData(t, res)
	}
}

func verifyTestData(t *testing.T, ms Series) {
	assert.Equal(t, 410, len(ms))
	assert.Equal(t, "go_gc_duration_seconds", ms[0].Labels.Get("__name__"))
	assert.Equal(t, "0.25", ms[0].Labels.Get("quantile"))
	assert.InDelta(t, 4.9351e-05, ms[0].Value, 0.0001)

	notExistYet := ms.FindByName("not_exist_yet")
	assert.NotNil(t, notExistYet)
	assert.Len(t, notExistYet, 0)

	targetInterval := ms.FindByName("prometheus_target_interval_length_seconds")
	assert.Len(t, targetInterval, 5)
}

func findScrapeModelSample(samples []promscrapemodel.Sample, name string, labelsMatch map[string]string) (promscrapemodel.Sample, bool) {
	for _, sample := range promscrapemodel.Samples(samples).FindByName(name) {
		if sample.Name != name {
			continue
		}

		matched := true
		for key, value := range labelsMatch {
			if sample.Labels.Get(key) != value {
				matched = false
				break
			}
		}
		if matched {
			return sample, true
		}
	}

	return promscrapemodel.Sample{}, false
}
