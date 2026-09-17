// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"
	"maps"
	"time"
)

func (c *protocolClient) Collect(ctx context.Context) (result collectionResult, err error) {
	started := time.Now()
	stats := &wireStats{
		failures: make(map[string]int),
	}
	result = collectionResult{
		ObservedAt: time.Now().UTC(),
		Metrics: cycleMetrics{
			Failures:     make(map[string]int),
			HTTPRequests: make(map[string]int),
			Operations:   make(map[string]int),
			Resources:    make(map[string]int),
		},
	}
	defer func() {
		result.Metrics.Duration = time.Since(started).Seconds()
	}()

	// Recover once after session expiry, after all reads have settled and before
	// projecting hardware or advancing counter baselines. Wire totals include both attempts.
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
		if identityIntegrityError(err) {
			break
		}
	}
	if graph == nil {
		result.Metrics.Status = "unavailable"
		c.copyWireStats(&result.Metrics, stats)
		return result, err
	}
	if identityIntegrityError(err) {
		result.Complete = false
		result.Diagnostics = append(result.Diagnostics, graph.finalDiagnostics()...)
		result.Diagnostics = append(result.Diagnostics, boundedDiagnostic(err.Error()))
		c.finishCollectionResult(&result, graph, stats, started)
		return result, err
	}
	var hardwareErr error
	result.Hardware, hardwareErr = c.hardwareSurface(graph, result.ObservedAt)
	err = errors.Join(err, hardwareErr)
	result.Complete = result.Complete && hardwareErr == nil
	result.Diagnostics = append(result.Diagnostics, graph.finalDiagnostics()...)
	if hardwareErr != nil {
		result.Diagnostics = append(result.Diagnostics, boundedDiagnostic(
			"Redfish metric surface: "+hardwareErr.Error(),
		))
	}
	if identityIntegrityError(hardwareErr) {
		result.Hardware = nil
		result.Complete = false
		c.finishCollectionResult(&result, graph, stats, started)
		return result, err
	}
	c.finishCollectionResult(&result, graph, stats, started)
	return result, err
}

func (c *protocolClient) acquireResourceGraph(ctx context.Context, stats *wireStats) (*resourceGraph, bool, error) {
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

func (c *protocolClient) finishCollectionResult(
	result *collectionResult,
	graph *resourceGraph,
	stats *wireStats,
	started time.Time,
) {
	if graph != nil {
		for _, resource := range graph.emittedNodes() {
			switch resource.AcquisitionState {
			case "readable":
				result.Metrics.Resources["readable"]++
			case "unreadable":
				result.Metrics.Resources["unreadable"]++
			default:
				result.Metrics.Resources["unknown"]++
			}
		}
	}
	result.Metrics.Resources["discovered"] = result.Metrics.Resources["readable"] +
		result.Metrics.Resources["unreadable"] + result.Metrics.Resources["unknown"]
	if result.Complete {
		result.Metrics.Status = "success"
	} else {
		result.Metrics.Status = "partial"
	}
	result.Metrics.Duration = time.Since(started).Seconds()
	c.copyWireStats(&result.Metrics, stats)
}

func (c *protocolClient) copyWireStats(metrics *cycleMetrics, stats *wireStats) {
	metrics.HTTPRequests["started"] = stats.started
	metrics.HTTPRequests["redirected"] = stats.redirected
	metrics.Operations["successful"] = stats.successful
	metrics.Operations["failed"] = stats.failed
	metrics.ReceivedBytes = stats.received
	maps.Copy(metrics.Failures, stats.failures)
}
