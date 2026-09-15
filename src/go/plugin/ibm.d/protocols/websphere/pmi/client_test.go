package pmi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestClientFetchSuccess(t *testing.T) {
	sampleXML := `<?xml version="1.0" encoding="UTF-8"?>
<PerformanceMonitor responseStatus="ok" version="8.5.5">
  <Node name="Node01">
    <Server name="server1">
      <Stat name="JVM">
        <CountStatistic name="Requests" count="10" unit="count"/>
        <Stat name="Nested">
          <TimeStatistic name="Response" count="2" totalTime="100" unit="ms"/>
        </Stat>
      </Stat>
    </Server>
  </Node>
  <Stat name="libertyRoot">
    <DoubleStatistic name="cpu" double="0.5" unit="percent"/>
  </Stat>
</PerformanceMonitor>`

	var capturedQuery string
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		capturedQuery = r.URL.RawQuery
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(sampleXML)),
			Header:     make(http.Header),
		}, nil
	})

	client, err := NewClientWithHTTP(Config{URL: "https://example.com/wasPerfTool/servlet/perfservlet", StatsType: "extended"}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := client.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if capturedQuery != "stats=extended" {
		t.Fatalf("expected stats query to enforce level, got %q", capturedQuery)
	}

	if snapshot.ResponseStatus != "ok" {
		t.Fatalf("unexpected response status: %s", snapshot.ResponseStatus)
	}
	if snapshot.Version != "8.5.5" {
		t.Fatalf("unexpected version: %s", snapshot.Version)
	}

	if len(snapshot.Nodes) != 1 {
		t.Fatalf("expected one node, got %d", len(snapshot.Nodes))
	}

	stat := snapshot.Nodes[0].Servers[0].Stats[0]
	if stat.Path != "Node01/server1/JVM" {
		t.Fatalf("unexpected stat path: %s", stat.Path)
	}
	if stat.CountStatistic == nil || stat.CountStatistic.Name != "Requests" {
		t.Fatalf("count statistic not normalised")
	}

	nested := stat.SubStats[0]
	if nested.Path != "Node01/server1/JVM/Nested" {
		t.Fatalf("unexpected nested stat path: %s", nested.Path)
	}
	if nested.TimeStatistic == nil || nested.TimeStatistic.Name != "Response" {
		t.Fatalf("time statistic not normalised")
	}

	rootStat := snapshot.Stats[0]
	if rootStat.Path != "libertyRoot" {
		t.Fatalf("unexpected root stat path: %s", rootStat.Path)
	}
	if rootStat.DoubleStatistic == nil || rootStat.DoubleStatistic.Name != "cpu" {
		t.Fatalf("double statistic not normalised")
	}
}

func TestClientFetchHTTPError(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("boom")),
		}, nil
	})

	client, err := NewClientWithHTTP(Config{URL: "https://example.com/wasPerfTool/servlet/perfservlet"}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := client.Fetch(ctx); err == nil {
		t.Fatalf("expected fetch error for 500 response")
	}
}

func TestClientFetchContextCancellation(t *testing.T) {
	httpClient := newTestHTTPClient(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})

	client, err := NewClientWithHTTP(Config{URL: "https://example.com/wasPerfTool/servlet/perfservlet"}, httpClient)
	if err != nil {
		t.Fatalf("NewClientWithHTTP failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Fetch(ctx); err == nil {
		t.Fatalf("expected fetch cancellation error")
	}
}
