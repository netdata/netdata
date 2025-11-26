// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"sync"
	"time"
)

// DataPoint represents a single metric data point
type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// Dimension represents a metric dimension within a chart
type Dimension struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Algorithm  string      `json:"algorithm"` // absolute, incremental, percentage-of-absolute-row
	Multiplier float64     `json:"multiplier"`
	Divisor    float64     `json:"divisor"`
	DataPoints []DataPoint `json:"data_points,omitempty"`
	LastValue  float64     `json:"last_value"`
	mu         sync.RWMutex
}

// Chart represents a collection of related dimensions
type Chart struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Name       string                `json:"name"`
	Title      string                `json:"title"`
	Units      string                `json:"units"`
	Family     string                `json:"family"`
	Context    string                `json:"context"`
	ChartType  string                `json:"chart_type"` // line, area, stacked
	Priority   int                   `json:"priority"`
	Dimensions map[string]*Dimension `json:"dimensions"`
	UpdateEvery int                  `json:"update_every"`
	LastUpdate time.Time             `json:"last_update"`
	mu         sync.RWMutex
}

// Node represents a monitored host/application
type Node struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Hostname    string            `json:"hostname"`
	OS          string            `json:"os"`
	Version     string            `json:"version"`
	Labels      map[string]string `json:"labels"`
	Charts      map[string]*Chart `json:"charts"`
	FirstSeen   time.Time         `json:"first_seen"`
	LastSeen    time.Time         `json:"last_seen"`
	Online      bool              `json:"online"`
	mu          sync.RWMutex
}

// MetricPush represents incoming metrics from a node
type MetricPush struct {
	NodeID     string            `json:"node_id"`
	NodeName   string            `json:"node_name,omitempty"`
	Hostname   string            `json:"hostname,omitempty"`
	OS         string            `json:"os,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Charts     []ChartPush       `json:"charts"`
	Timestamp  int64             `json:"timestamp,omitempty"` // Unix timestamp in milliseconds
}

// ChartPush represents a chart in an incoming push
type ChartPush struct {
	ID         string          `json:"id"`
	Type       string          `json:"type,omitempty"`
	Name       string          `json:"name,omitempty"`
	Title      string          `json:"title,omitempty"`
	Units      string          `json:"units,omitempty"`
	Family     string          `json:"family,omitempty"`
	Context    string          `json:"context,omitempty"`
	ChartType  string          `json:"chart_type,omitempty"`
	Priority   int             `json:"priority,omitempty"`
	UpdateEvery int            `json:"update_every,omitempty"`
	Dimensions []DimensionPush `json:"dimensions"`
}

// DimensionPush represents a dimension in an incoming push
type DimensionPush struct {
	ID         string  `json:"id"`
	Name       string  `json:"name,omitempty"`
	Algorithm  string  `json:"algorithm,omitempty"`
	Multiplier float64 `json:"multiplier,omitempty"`
	Divisor    float64 `json:"divisor,omitempty"`
	Value      float64 `json:"value"`
}

// WebSocketMessage represents messages sent to connected clients
type WebSocketMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// NodeSummary is a lightweight node representation for listings
type NodeSummary struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Hostname   string            `json:"hostname"`
	OS         string            `json:"os"`
	Labels     map[string]string `json:"labels"`
	ChartCount int               `json:"chart_count"`
	LastSeen   time.Time         `json:"last_seen"`
	Online     bool              `json:"online"`
}

// ChartData represents chart data for API responses
type ChartData struct {
	Chart      *Chart    `json:"chart"`
	Labels     []int64   `json:"labels"`     // Timestamps
	Data       [][]float64 `json:"data"`     // Dimension values
}

// QueryParams for data API requests
type QueryParams struct {
	NodeID   string   `json:"node_id"`
	ChartID  string   `json:"chart_id"`
	After    int64    `json:"after"`    // Unix timestamp
	Before   int64    `json:"before"`   // Unix timestamp
	Points   int      `json:"points"`   // Number of points to return
	Dims     []string `json:"dims"`     // Specific dimensions
}
