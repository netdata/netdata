// SPDX-License-Identifier: GPL-3.0-or-later

package typesense

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathHealth = "/health"
	urlPathStats  = "/stats.json"
)

// https://typesense.org/docs/27.0/api/cluster-operations.html#health
type healthResponse struct {
	Ok  *bool  `json:"ok"`
	Err string `json:"resource_error"`
}

// https://typesense.org/docs/27.0/api/cluster-operations.html#api-stats
type statsResponse struct {
	DeleteLatencyMs             float64 `json:"delete_latency_ms" stm:"delete_latency_ms"`
	DeleteRequestsPerSecond     float64 `json:"delete_requests_per_second" stm:"delete_requests_per_second,1000,1"`
	ImportLatencyMs             float64 `json:"import_latency_ms" stm:"import_latency_ms"`
	ImportRequestsPerSecond     float64 `json:"import_requests_per_second" stm:"import_requests_per_second,1000,1"`
	OverloadedRequestsPerSecond float64 `json:"overloaded_requests_per_second" stm:"overloaded_requests_per_second,1000,1"`
	PendingWriteBatches         float64 `json:"pending_write_batches" stm:"pending_write_batches"`
	SearchLatencyMs             float64 `json:"search_latency_ms" stm:"search_latency_ms"`
	SearchRequestsPerSecond     float64 `json:"search_requests_per_second" stm:"search_requests_per_second,1000,1"`
	TotalRequestsPerSecond      float64 `json:"total_requests_per_second" stm:"total_requests_per_second,1000,1"`
	WriteLatencyMs              float64 `json:"write_latency_ms" stm:"write_latency_ms"`
	WriteRequestsPerSecond      float64 `json:"write_requests_per_second" stm:"write_requests_per_second,1000,1"`
}

func (ts *Typesense) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := ts.collectHealth(mx); err != nil {
		return nil, err
	}

	if err := ts.collectStats(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (ts *Typesense) collectHealth(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(ts.Request, urlPathHealth)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}

	var resp healthResponse
	if err := ts.doOKDecode(req, &resp); err != nil {
		return err
	}

	px := "health_status_"

	for _, v := range []string{"ok", "out_of_disk", "out_of_memory"} {
		mx[px+v] = 0
	}

	if resp.Ok == nil {
		return fmt.Errorf("unexpected response: no health status found")
	}

	if resp.Err != "" {
		mx[px+strings.ToLower(resp.Err)] = 1
	} else if *resp.Ok {
		mx[px+"ok"] = 1
	}

	return nil
}

func (ts *Typesense) collectStats(mx map[string]int64) error {
	if !ts.doStats || ts.APIKey == "" {
		return nil
	}

	req, err := web.NewHTTPRequestWithPath(ts.Request, urlPathStats)
	if err != nil {
		return fmt.Errorf("creating stats request: %w", err)
	}

	req.Header.Set("X-TYPESENSE-API-KEY", ts.APIKey)

	var resp statsResponse
	if err := ts.doOKDecode(req, &resp); err != nil {
		if !isStatusUnauthorized(err) {
			return err
		}

		ts.doStats = false
		ts.Warning(err)

		return nil
	}

	ts.once.Do(ts.addStatsCharts)

	for k, v := range stm.ToMap(resp) {
		mx[k] = v
	}

	return nil
}

func (ts *Typesense) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := ts.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		// {"message": "Forbidden - a valid `x-typesense-api-key` header must be sent."}
		var msg struct {
			Msg string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msg); err == nil {
			return fmt.Errorf("'%s' returned HTTP status code: %d (msg: '%s')", req.URL, resp.StatusCode, msg.Msg)
		}
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

func isStatusUnauthorized(err error) bool {
	return strings.Contains(err.Error(), "code: 401")
}
