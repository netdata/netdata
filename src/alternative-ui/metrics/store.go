// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"sync"
	"time"
)

const (
	// DefaultRetention is the default data retention period
	DefaultRetention = 1 * time.Hour
	// DefaultMaxPoints is the maximum points per dimension
	DefaultMaxPoints = 3600
	// NodeTimeoutDuration is how long before a node is considered offline
	NodeTimeoutDuration = 30 * time.Second
)

// Store is the central metrics storage
type Store struct {
	nodes       map[string]*Node
	mu          sync.RWMutex
	retention   time.Duration
	maxPoints   int
	subscribers []chan WebSocketMessage
	subMu       sync.RWMutex
}

// NewStore creates a new metrics store
func NewStore(retention time.Duration, maxPoints int) *Store {
	if retention == 0 {
		retention = DefaultRetention
	}
	if maxPoints == 0 {
		maxPoints = DefaultMaxPoints
	}

	s := &Store{
		nodes:       make(map[string]*Node),
		retention:   retention,
		maxPoints:   maxPoints,
		subscribers: make([]chan WebSocketMessage, 0),
	}

	// Start cleanup goroutine
	go s.cleanupLoop()
	// Start node status checker
	go s.nodeStatusLoop()

	return s
}

// Subscribe adds a new WebSocket subscriber
func (s *Store) Subscribe() chan WebSocketMessage {
	ch := make(chan WebSocketMessage, 100)
	s.subMu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.subMu.Unlock()
	return ch
}

// Unsubscribe removes a WebSocket subscriber
func (s *Store) Unsubscribe(ch chan WebSocketMessage) {
	s.subMu.Lock()
	defer s.subMu.Unlock()

	for i, sub := range s.subscribers {
		if sub == ch {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			close(ch)
			return
		}
	}
}

// broadcast sends a message to all subscribers
func (s *Store) broadcast(msg WebSocketMessage) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	for _, ch := range s.subscribers {
		select {
		case ch <- msg:
		default:
			// Channel full, skip
		}
	}
}

// Push processes incoming metrics from a node
func (s *Store) Push(push *MetricPush) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ts := now
	if push.Timestamp > 0 {
		ts = time.UnixMilli(push.Timestamp)
	}

	// Get or create node
	node, exists := s.nodes[push.NodeID]
	if !exists {
		node = &Node{
			ID:        push.NodeID,
			Name:      push.NodeName,
			Hostname:  push.Hostname,
			OS:        push.OS,
			Labels:    push.Labels,
			Charts:    make(map[string]*Chart),
			FirstSeen: now,
			Online:    true,
		}
		if node.Name == "" {
			node.Name = push.NodeID
		}
		s.nodes[push.NodeID] = node

		// Notify subscribers of new node
		s.broadcast(WebSocketMessage{
			Type:    "node_added",
			Payload: s.nodeSummary(node),
		})
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	node.LastSeen = now
	node.Online = true
	if push.NodeName != "" {
		node.Name = push.NodeName
	}
	if push.Hostname != "" {
		node.Hostname = push.Hostname
	}
	if push.OS != "" {
		node.OS = push.OS
	}
	if push.Labels != nil {
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		for k, v := range push.Labels {
			node.Labels[k] = v
		}
	}

	// Process charts
	for _, chartPush := range push.Charts {
		chart, exists := node.Charts[chartPush.ID]
		if !exists {
			chart = &Chart{
				ID:          chartPush.ID,
				Type:        chartPush.Type,
				Name:        chartPush.Name,
				Title:       chartPush.Title,
				Units:       chartPush.Units,
				Family:      chartPush.Family,
				Context:     chartPush.Context,
				ChartType:   chartPush.ChartType,
				Priority:    chartPush.Priority,
				UpdateEvery: chartPush.UpdateEvery,
				Dimensions:  make(map[string]*Dimension),
			}
			if chart.Name == "" {
				chart.Name = chart.ID
			}
			if chart.Title == "" {
				chart.Title = chart.Name
			}
			if chart.ChartType == "" {
				chart.ChartType = "line"
			}
			if chart.UpdateEvery == 0 {
				chart.UpdateEvery = 1
			}
			node.Charts[chartPush.ID] = chart
		}

		chart.mu.Lock()
		chart.LastUpdate = now

		// Update chart metadata if provided
		if chartPush.Title != "" {
			chart.Title = chartPush.Title
		}
		if chartPush.Units != "" {
			chart.Units = chartPush.Units
		}

		// Process dimensions
		for _, dimPush := range chartPush.Dimensions {
			dim, exists := chart.Dimensions[dimPush.ID]
			if !exists {
				dim = &Dimension{
					ID:         dimPush.ID,
					Name:       dimPush.Name,
					Algorithm:  dimPush.Algorithm,
					Multiplier: dimPush.Multiplier,
					Divisor:    dimPush.Divisor,
					DataPoints: make([]DataPoint, 0, s.maxPoints),
				}
				if dim.Name == "" {
					dim.Name = dim.ID
				}
				if dim.Algorithm == "" {
					dim.Algorithm = "absolute"
				}
				if dim.Multiplier == 0 {
					dim.Multiplier = 1
				}
				if dim.Divisor == 0 {
					dim.Divisor = 1
				}
				chart.Dimensions[dimPush.ID] = dim
			}

			dim.mu.Lock()
			dim.LastValue = dimPush.Value
			dim.DataPoints = append(dim.DataPoints, DataPoint{
				Timestamp: ts,
				Value:     dimPush.Value,
			})

			// Trim to max points
			if len(dim.DataPoints) > s.maxPoints {
				dim.DataPoints = dim.DataPoints[len(dim.DataPoints)-s.maxPoints:]
			}
			dim.mu.Unlock()
		}
		chart.mu.Unlock()
	}

	// Broadcast update to subscribers
	s.broadcast(WebSocketMessage{
		Type: "metrics_update",
		Payload: map[string]interface{}{
			"node_id":   push.NodeID,
			"timestamp": ts.UnixMilli(),
			"charts":    push.Charts,
		},
	})

	return nil
}

// GetNodes returns a summary of all nodes
func (s *Store) GetNodes() []NodeSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nodes := make([]NodeSummary, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, s.nodeSummary(node))
	}
	return nodes
}

// nodeSummary creates a NodeSummary from a Node
func (s *Store) nodeSummary(node *Node) NodeSummary {
	node.mu.RLock()
	defer node.mu.RUnlock()

	return NodeSummary{
		ID:         node.ID,
		Name:       node.Name,
		Hostname:   node.Hostname,
		OS:         node.OS,
		Labels:     node.Labels,
		ChartCount: len(node.Charts),
		LastSeen:   node.LastSeen,
		Online:     node.Online,
	}
}

// GetNode returns a specific node by ID
func (s *Store) GetNode(nodeID string) (*Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	node, ok := s.nodes[nodeID]
	return node, ok
}

// GetCharts returns all charts for a node
func (s *Store) GetCharts(nodeID string) ([]*Chart, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return nil, false
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	charts := make([]*Chart, 0, len(node.Charts))
	for _, chart := range node.Charts {
		charts = append(charts, chart)
	}
	return charts, true
}

// GetChartData returns data for a specific chart
func (s *Store) GetChartData(nodeID, chartID string, after, before int64, points int) (*ChartData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[nodeID]
	if !ok {
		return nil, false
	}

	node.mu.RLock()
	chart, ok := node.Charts[chartID]
	node.mu.RUnlock()
	if !ok {
		return nil, false
	}

	chart.mu.RLock()
	defer chart.mu.RUnlock()

	// Determine time range
	if before == 0 {
		before = time.Now().UnixMilli()
	}
	if after == 0 {
		after = before - int64(s.retention.Milliseconds())
	}

	afterTime := time.UnixMilli(after)
	beforeTime := time.UnixMilli(before)

	// Collect timestamps and data
	timestamps := make(map[int64]int) // timestamp -> index
	var labels []int64

	// First pass: collect all unique timestamps
	for _, dim := range chart.Dimensions {
		dim.mu.RLock()
		for _, dp := range dim.DataPoints {
			if dp.Timestamp.After(afterTime) && dp.Timestamp.Before(beforeTime) {
				ts := dp.Timestamp.UnixMilli()
				if _, exists := timestamps[ts]; !exists {
					timestamps[ts] = len(labels)
					labels = append(labels, ts)
				}
			}
		}
		dim.mu.RUnlock()
	}

	// Sort labels (they should be roughly sorted already)
	// Simple bubble sort for small datasets
	for i := 0; i < len(labels)-1; i++ {
		for j := 0; j < len(labels)-i-1; j++ {
			if labels[j] > labels[j+1] {
				labels[j], labels[j+1] = labels[j+1], labels[j]
			}
		}
	}

	// Rebuild timestamp index after sorting
	for i, ts := range labels {
		timestamps[ts] = i
	}

	// Second pass: collect data
	dimNames := make([]string, 0, len(chart.Dimensions))
	data := make([][]float64, 0, len(chart.Dimensions))

	for dimID, dim := range chart.Dimensions {
		dimNames = append(dimNames, dimID)
		dimData := make([]float64, len(labels))
		// Initialize with NaN-like placeholder (0 for simplicity)
		for i := range dimData {
			dimData[i] = 0
		}

		dim.mu.RLock()
		for _, dp := range dim.DataPoints {
			ts := dp.Timestamp.UnixMilli()
			if idx, ok := timestamps[ts]; ok {
				dimData[idx] = dp.Value
			}
		}
		dim.mu.RUnlock()

		data = append(data, dimData)
	}

	// Downsample if needed
	if points > 0 && len(labels) > points {
		labels, data = downsample(labels, data, points)
	}

	return &ChartData{
		Chart:  chart,
		Labels: labels,
		Data:   data,
	}, true
}

// downsample reduces the number of data points
func downsample(labels []int64, data [][]float64, targetPoints int) ([]int64, [][]float64) {
	if len(labels) <= targetPoints {
		return labels, data
	}

	step := float64(len(labels)) / float64(targetPoints)
	newLabels := make([]int64, targetPoints)
	newData := make([][]float64, len(data))
	for i := range newData {
		newData[i] = make([]float64, targetPoints)
	}

	for i := 0; i < targetPoints; i++ {
		idx := int(float64(i) * step)
		if idx >= len(labels) {
			idx = len(labels) - 1
		}
		newLabels[i] = labels[idx]
		for j := range data {
			newData[j][i] = data[j][idx]
		}
	}

	return newLabels, newData
}

// cleanupLoop periodically removes old data points
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes data points older than retention period
func (s *Store) cleanup() {
	s.mu.RLock()
	nodes := make([]*Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()

	cutoff := time.Now().Add(-s.retention)

	for _, node := range nodes {
		node.mu.RLock()
		charts := make([]*Chart, 0, len(node.Charts))
		for _, chart := range node.Charts {
			charts = append(charts, chart)
		}
		node.mu.RUnlock()

		for _, chart := range charts {
			chart.mu.RLock()
			dims := make([]*Dimension, 0, len(chart.Dimensions))
			for _, dim := range chart.Dimensions {
				dims = append(dims, dim)
			}
			chart.mu.RUnlock()

			for _, dim := range dims {
				dim.mu.Lock()
				newPoints := make([]DataPoint, 0, len(dim.DataPoints))
				for _, dp := range dim.DataPoints {
					if dp.Timestamp.After(cutoff) {
						newPoints = append(newPoints, dp)
					}
				}
				dim.DataPoints = newPoints
				dim.mu.Unlock()
			}
		}
	}
}

// nodeStatusLoop periodically checks node online status
func (s *Store) nodeStatusLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.checkNodeStatus()
	}
}

// checkNodeStatus marks nodes as offline if they haven't sent data recently
func (s *Store) checkNodeStatus() {
	s.mu.RLock()
	nodes := make([]*Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		nodes = append(nodes, node)
	}
	s.mu.RUnlock()

	cutoff := time.Now().Add(-NodeTimeoutDuration)

	for _, node := range nodes {
		node.mu.Lock()
		wasOnline := node.Online
		node.Online = node.LastSeen.After(cutoff)
		isOnline := node.Online
		node.mu.Unlock()

		if wasOnline && !isOnline {
			s.broadcast(WebSocketMessage{
				Type: "node_offline",
				Payload: map[string]string{
					"node_id": node.ID,
				},
			})
		} else if !wasOnline && isOnline {
			s.broadcast(WebSocketMessage{
				Type: "node_online",
				Payload: map[string]string{
					"node_id": node.ID,
				},
			})
		}
	}
}
