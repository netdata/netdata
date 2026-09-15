package openmetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestFetchSeries(t *testing.T) {
	payload := strings.Join([]string{
		"# HELP test_metric A test metric",
		"# TYPE test_metric gauge",
		"test_metric{label=\"value\"} 42",
		"another_metric 7",
	}, "\n")

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Accept") == "" {
			t.Fatalf("expected Accept header to be set")
		}
		if r.Header.Get("Accept-Encoding") != "gzip" {
			t.Fatalf("expected Accept-Encoding gzip header")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
			Header:     make(http.Header),
		}, nil
	})

	client, err := NewClientWithHTTP(Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://example.com/metrics"}}}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	series, err := client.FetchSeries(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchSeries failed: %v", err)
	}

	if len(series) != 2 {
		t.Fatalf("expected 2 series entries, got %d", len(series))
	}

	if series[0].Name() != "another_metric" || series[0].Value != 7 {
		t.Fatalf("unexpected first sample: %+v", series[0])
	}

	if series[1].Name() != "test_metric" || series[1].Value != 42 {
		t.Fatalf("unexpected second sample: %+v", series[1])
	}

	if got := series[1].Labels.Get("label"); got != "value" {
		t.Fatalf("expected label \"value\", got %q", got)
	}
}

func TestFetchSeriesGzip(t *testing.T) {
	raw := strings.Join([]string{
		"metric_one 1",
		"metric_two 2",
	}, "\n")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(raw)); err != nil {
		t.Fatalf("failed to compress payload: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
			Header:     make(http.Header),
		}
		resp.Header.Set("Content-Encoding", "gzip")
		return resp, nil
	})

	client, err := NewClientWithHTTP(Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://example.com/metrics"}}}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	series, err := client.FetchSeries(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchSeries failed: %v", err)
	}

	if len(series) != 2 {
		t.Fatalf("expected 2 series entries, got %d", len(series))
	}
}

func TestFetchSeriesHTTPError(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	})

	client, err := NewClientWithHTTP(Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://example.com/metrics"}}}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	if _, err := client.FetchSeries(context.Background(), nil); err == nil {
		t.Fatalf("expected error for HTTP status")
	}
}

func TestFetchSeriesCancellation(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})

	client, err := NewClientWithHTTP(Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://example.com/metrics"}}}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.FetchSeries(ctx, nil); err == nil {
		t.Fatalf("expected cancellation error")
	}
}
