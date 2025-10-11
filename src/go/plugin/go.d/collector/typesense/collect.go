// SPDX-License-Identifier: GPL-3.0-or-later

package typesense

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
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

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectHealth(mx); err != nil {
		return nil, err
	}

	if err := c.collectStats(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectHealth(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathHealth)
	if err != nil {
		return fmt.Errorf("creating health request: %w", err)
	}

	var resp healthResponse
	if err := c.client().RequestJSON(req, &resp); err != nil {
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

func (c *Collector) collectStats(mx map[string]int64) error {
	if !c.doStats || c.APIKey == "" {
		return nil
	}

	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStats)
	if err != nil {
		return fmt.Errorf("creating stats request: %w", err)
	}

	req.Header.Set("X-TYPESENSE-API-KEY", c.APIKey)

	var resp statsResponse
	if err := c.client().RequestJSON(req, &resp); err != nil {
		if !strings.Contains(err.Error(), "code: 401") {
			return err
		}

		c.doStats = false
		c.Warning(err)

		return nil
	}

	c.once.Do(c.addStatsCharts)

	for k, v := range stm.ToMap(resp) {
		mx[k] = v
	}

	return nil
}

func (c *Collector) client() *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		// {"message": "Forbidden - a valid `x-typesense-api-key` header must be sent."}
		var msg struct {
			Msg string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msg); err == nil && msg.Msg != "" {
			return false, fmt.Errorf("msg: '%s'", msg.Msg)
		}
		return false, nil
	})
}
