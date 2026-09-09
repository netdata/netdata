// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataStatusMetrics, _        = os.ReadFile("testdata/status.txt")
	dataTengineStatusMetrics, _ = os.ReadFile("testdata/tengine-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataStatusMetrics":        dataStatusMetrics,
		"dataTengineStatusMetrics": dataTengineStatusMetrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	require.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_CheckNG(t *testing.T) {
	collr := New()

	collr.URL = "http://127.0.0.1:38001/us"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_LifecycleContextCancelsHTTPRequest(t *testing.T) {
	tests := map[string]func(context.Context, *Collector) error{
		"check": func(ctx context.Context, collector *Collector) error {
			return collector.Check(ctx)
		},
		"collect": func(ctx context.Context, collector *Collector) error {
			collector.Collect(ctx)
			return nil
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			requestCanceled := make(chan struct{})
			releaseServer := make(chan struct{})
			ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				close(entered)
				select {
				case <-req.Context().Done():
					close(requestCanceled)
				case <-releaseServer:
				}
			}))
			defer ts.Close()

			collector := New()
			collector.URL = ts.URL
			collector.Timeout = confopt.Duration(time.Minute)
			require.NoError(t, collector.Init(context.Background()))

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- run(ctx, collector)
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "HTTP request was not received")
			}
			cancel()
			select {
			case <-requestCanceled:
				close(releaseServer)
			case <-time.After(time.Second):
				close(releaseServer)
				require.FailNow(t, "test failed", "HTTP request ignored lifecycle cancellation")
			}
			select {
			case err := <-done:
				if name == "check" {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "collector ignored lifecycle cancellation")
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"accepts":  36,
		"active":   1,
		"handled":  36,
		"reading":  0,
		"requests": 126,
		"waiting":  0,
		"writing":  1,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_CollectTengine(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataTengineStatusMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"accepts":      1140,
		"active":       1,
		"handled":      1140,
		"reading":      0,
		"request_time": 75806,
		"requests":     1140,
		"waiting":      0,
		"writing":      1,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_InvalidData(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
