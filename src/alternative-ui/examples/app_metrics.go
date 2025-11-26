// SPDX-License-Identifier: GPL-3.0-or-later

// Example: Push application metrics to Netdata Alternative UI
//
// This example shows how to instrument a Go application to push custom
// metrics to the alternative UI server.
//
// Build and run:
//   go build -o app_metrics app_metrics.go
//   ./app_metrics --url http://localhost:19998 --node-id my-app

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"time"
)

// MetricPush is the payload sent to the server
type MetricPush struct {
	NodeID    string            `json:"node_id"`
	NodeName  string            `json:"node_name,omitempty"`
	Hostname  string            `json:"hostname,omitempty"`
	OS        string            `json:"os,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Charts    []Chart           `json:"charts"`
	Timestamp int64             `json:"timestamp"`
}

// Chart represents a metric chart
type Chart struct {
	ID          string      `json:"id"`
	Type        string      `json:"type,omitempty"`
	Name        string      `json:"name,omitempty"`
	Title       string      `json:"title,omitempty"`
	Units       string      `json:"units,omitempty"`
	Family      string      `json:"family,omitempty"`
	Context     string      `json:"context,omitempty"`
	ChartType   string      `json:"chart_type,omitempty"`
	Priority    int         `json:"priority,omitempty"`
	UpdateEvery int         `json:"update_every,omitempty"`
	Dimensions  []Dimension `json:"dimensions"`
}

// Dimension represents a metric dimension
type Dimension struct {
	ID         string  `json:"id"`
	Name       string  `json:"name,omitempty"`
	Algorithm  string  `json:"algorithm,omitempty"`
	Multiplier float64 `json:"multiplier,omitempty"`
	Divisor    float64 `json:"divisor,omitempty"`
	Value      float64 `json:"value"`
}

// MetricsPusher pushes metrics to the server
type MetricsPusher struct {
	URL      string
	NodeID   string
	NodeName string
	APIKey   string
	client   *http.Client
}

// NewMetricsPusher creates a new metrics pusher
func NewMetricsPusher(url, nodeID, nodeName, apiKey string) *MetricsPusher {
	return &MetricsPusher{
		URL:      url,
		NodeID:   nodeID,
		NodeName: nodeName,
		APIKey:   apiKey,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Push sends metrics to the server
func (p *MetricsPusher) Push(charts []Chart) error {
	hostname, _ := os.Hostname()

	payload := MetricPush{
		NodeID:    p.NodeID,
		NodeName:  p.NodeName,
		Hostname:  hostname,
		OS:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Timestamp: time.Now().UnixMilli(),
		Charts:    charts,
		Labels: map[string]string{
			"go_version": runtime.Version(),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", p.URL+"/api/v1/push", bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("X-API-Key", p.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// SimulatedApp represents a simulated application with metrics
type SimulatedApp struct {
	requestCount    int64
	requestLatency  float64
	activeUsers     int
	errorCount      int64
	cacheHits       int64
	cacheMisses     int64
	queueSize       int
	processingRate  float64
}

// Update simulates application activity
func (a *SimulatedApp) Update() {
	// Simulate request patterns
	a.requestCount += int64(rand.Intn(100) + 50)
	a.requestLatency = 10 + rand.Float64()*50 + float64(a.activeUsers)*0.1

	// Simulate user activity
	if rand.Float32() > 0.5 {
		a.activeUsers += rand.Intn(5)
	} else {
		a.activeUsers -= rand.Intn(3)
	}
	if a.activeUsers < 10 {
		a.activeUsers = 10
	}
	if a.activeUsers > 500 {
		a.activeUsers = 500
	}

	// Simulate errors
	if rand.Float32() > 0.95 {
		a.errorCount += int64(rand.Intn(3))
	}

	// Simulate cache
	if rand.Float32() > 0.3 {
		a.cacheHits += int64(rand.Intn(50))
	} else {
		a.cacheMisses += int64(rand.Intn(10))
	}

	// Simulate queue
	a.queueSize = rand.Intn(100)
	a.processingRate = 50 + rand.Float64()*50
}

// GetCharts returns the application metrics as charts
func (a *SimulatedApp) GetCharts() []Chart {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return []Chart{
		{
			ID:        "app.requests",
			Type:      "app",
			Title:     "HTTP Requests",
			Units:     "requests/s",
			Family:    "http",
			ChartType: "area",
			Priority:  100,
			Dimensions: []Dimension{
				{ID: "requests", Name: "Requests", Value: float64(a.requestCount), Algorithm: "incremental"},
				{ID: "errors", Name: "Errors", Value: float64(a.errorCount), Algorithm: "incremental"},
			},
		},
		{
			ID:        "app.latency",
			Type:      "app",
			Title:     "Request Latency",
			Units:     "ms",
			Family:    "http",
			ChartType: "line",
			Priority:  110,
			Dimensions: []Dimension{
				{ID: "avg_latency", Name: "Average", Value: a.requestLatency},
			},
		},
		{
			ID:        "app.users",
			Type:      "app",
			Title:     "Active Users",
			Units:     "users",
			Family:    "users",
			ChartType: "line",
			Priority:  200,
			Dimensions: []Dimension{
				{ID: "active", Name: "Active Users", Value: float64(a.activeUsers)},
			},
		},
		{
			ID:        "app.cache",
			Type:      "app",
			Title:     "Cache Performance",
			Units:     "operations/s",
			Family:    "cache",
			ChartType: "stacked",
			Priority:  300,
			Dimensions: []Dimension{
				{ID: "hits", Name: "Hits", Value: float64(a.cacheHits), Algorithm: "incremental"},
				{ID: "misses", Name: "Misses", Value: float64(a.cacheMisses), Algorithm: "incremental"},
			},
		},
		{
			ID:        "app.queue",
			Type:      "app",
			Title:     "Processing Queue",
			Units:     "items",
			Family:    "queue",
			ChartType: "area",
			Priority:  400,
			Dimensions: []Dimension{
				{ID: "queue_size", Name: "Queue Size", Value: float64(a.queueSize)},
				{ID: "processing_rate", Name: "Processing Rate", Value: a.processingRate},
			},
		},
		{
			ID:        "app.memory",
			Type:      "app",
			Title:     "Go Memory Usage",
			Units:     "MiB",
			Family:    "memory",
			ChartType: "stacked",
			Priority:  500,
			Dimensions: []Dimension{
				{ID: "heap_alloc", Name: "Heap Alloc", Value: float64(memStats.HeapAlloc) / 1024 / 1024},
				{ID: "heap_sys", Name: "Heap Sys", Value: float64(memStats.HeapSys) / 1024 / 1024},
				{ID: "stack_sys", Name: "Stack", Value: float64(memStats.StackSys) / 1024 / 1024},
			},
		},
		{
			ID:        "app.goroutines",
			Type:      "app",
			Title:     "Goroutines",
			Units:     "goroutines",
			Family:    "runtime",
			ChartType: "line",
			Priority:  510,
			Dimensions: []Dimension{
				{ID: "goroutines", Name: "Active", Value: float64(runtime.NumGoroutine())},
			},
		},
	}
}

func main() {
	url := flag.String("url", "http://localhost:19998", "Server URL")
	nodeID := flag.String("node-id", "go-app", "Node ID")
	nodeName := flag.String("node-name", "Go Application", "Node display name")
	apiKey := flag.String("api-key", "", "API key for authentication")
	interval := flag.Duration("interval", time.Second, "Push interval")
	flag.Parse()

	log.Printf("Pushing metrics to %s", *url)
	log.Printf("Node ID: %s", *nodeID)
	log.Printf("Node Name: %s", *nodeName)
	log.Printf("Interval: %s", *interval)

	pusher := NewMetricsPusher(*url, *nodeID, *nodeName, *apiKey)
	app := &SimulatedApp{activeUsers: 50}

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			app.Update()
			charts := app.GetCharts()

			if err := pusher.Push(charts); err != nil {
				log.Printf("Failed to push metrics: %v", err)
			} else {
				log.Printf("[%s] Metrics pushed successfully", time.Now().Format("15:04:05"))
			}
		}
	}
}
