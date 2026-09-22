// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

// Result owns one completed acquisition attempt. Resources contain current facts,
// including retained identities without stale readings. No traversal state is exposed.
type Result struct {
	Resources     []*measurement.Resource
	Available     bool // A resource graph was acquired, even if it is partial.
	Complete      bool // Overall acquisition succeeded, including base collections.
	GraphComplete bool // Authorizes measurement history pruning; distinct from Complete.
	Diagnostics   Diagnostics
	Statistics    Statistics
	AuthMethod    string
}

// Statistics contains source-operation accounting, including a discarded session
// attempt. Collection status and total duration belong to the collector.
type Statistics struct {
	Failures      map[string]int
	HTTPRequests  map[string]int
	Operations    map[string]int
	ReceivedBytes int64
	Resources     map[string]int
}

// Acquire settles all endpoint reads and at most one session recovery before
// returning facts. The caller may then project them exactly once.
func (c *Client) Acquire(ctx context.Context) (result Result, err error) {
	stats := &wireStats{
		failures: make(map[string]int),
	}
	result.Statistics = Statistics{
		Failures:     make(map[string]int),
		HTTPRequests: make(map[string]int),
		Operations:   make(map[string]int),
		Resources:    make(map[string]int),
	}
	defer func() {
		result.AuthMethod = c.authMode
		result.Statistics.HTTPRequests["started"] = stats.started
		result.Statistics.HTTPRequests["redirected"] = stats.redirected
		result.Statistics.Operations["successful"] = stats.successful
		result.Statistics.Operations["failed"] = stats.failed
		result.Statistics.ReceivedBytes = stats.received
		maps.Copy(result.Statistics.Failures, stats.failures)
	}()
	var graph *resourceGraph
	for range 2 {
		stats.unauthorized = false
		graph, result.Complete, err = c.acquireResourceGraph(ctx, stats)
		if c.authMode != "session" || !stats.unauthorized {
			break
		}
		c.closeSession(ctx)
		if ctx.Err() != nil {
			err = errors.Join(err, ctx.Err())
			break
		}
		if identity.IsIntegrityError(err) {
			break
		}
	}
	if graph == nil {
		return result, err
	}
	result.Available = true
	result.GraphComplete = graph.Complete
	result.Resources = graph.measurementResources()
	result.Diagnostics = graph.diagnostics
	for _, resource := range result.Resources {
		switch resource.AcquisitionState {
		case "readable":
			result.Statistics.Resources["readable"]++
		case "unreadable":
			result.Statistics.Resources["unreadable"]++
		default:
			result.Statistics.Resources["unknown"]++
		}
	}
	result.Statistics.Resources["discovered"] = result.Statistics.Resources["readable"] +
		result.Statistics.Resources["unreadable"] + result.Statistics.Resources["unknown"]
	return result, err
}

func (c *Client) acquireResourceGraph(ctx context.Context, stats *wireStats) (*resourceGraph, bool, error) {
	if err := c.initializeAuthentication(ctx, stats); err != nil {
		return nil, false, err
	}
	root, err := c.fetchServiceRoot(ctx, stats)
	if err != nil {
		return nil, false, err
	}
	resources, baseComplete, baseErr := c.fetchBaseResources(ctx, root, stats)
	graph, graphErr := c.collectResourceGraph(ctx, root, resources, stats)
	err = errors.Join(baseErr, graphErr)
	return graph, baseComplete && graph.Complete && err == nil, err
}
