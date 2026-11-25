// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"sync"
)

const (
	dnsFunctionName    = "dns-queries"
	dnsFunctionTimeout = 10
)

// dnsFunctionStore holds the latest computed per-cycle DNS metrics for the
// dns-queries function. Updated by the collector; read by function handlers.
// NOTE: DNS uses flow-based data (not global counters like socket), so this
// is a placeholder. Full implementation requires aggregating flow snapshots.
type dnsFunctionStore struct {
	mu      sync.RWMutex
	hasData bool
}

func newDNSFunctionStore() *dnsFunctionStore {
	return &dnsFunctionStore{}
}

func (s *dnsFunctionStore) update() {
	s.mu.Lock()
	s.hasData = true
	s.mu.Unlock()
}

func (s *dnsFunctionStore) hasMetrics() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasData
}

// handleDNSQueriesFunction serves the dns-queries function request.
// Returns a JSON response with DNS query statistics.
// Phase 5 follow-up: Implement full flow aggregation and table response.
func (c *DNSCollector) handleDNSQueriesFunction() (string, error) {
	if c.fnStore == nil {
		return "", fmt.Errorf("dns function store not initialized")
	}

	if !c.fnStore.hasMetrics() {
		return "", fmt.Errorf("dns: data not yet available")
	}

	// Placeholder response: TODO implement full dns-queries table
	// Should aggregate DNS flow snapshot data and return table with:
	// - Query/response counts
	// - Protocol stats (UDP vs TCP)
	// - Error rates
	// - Top domains/IPs (when available)
	resp := map[string]interface{}{
		"status": 200,
		"type":   "table",
		"data":   [][]interface{}{},
		"note":   "dns-queries function: phase 5 follow-up, full implementation pending",
	}

	data, _ := json.Marshal(resp)
	return string(data), nil
}
