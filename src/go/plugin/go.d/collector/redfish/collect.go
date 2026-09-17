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
		if c.authMode == "session" && stats.unauthorized {
			c.Close()
		}
		result.Metrics.Duration = time.Since(started).Seconds()
	}()

	err = c.initializeAuthentication(ctx, stats)
	var root *serviceRootDocument
	if err == nil {
		root, err = c.fetchServiceRoot(ctx, stats)
	}
	if err != nil {
		result.Metrics.Status = "unavailable"
		result.Metrics.Duration = time.Since(started).Seconds()
		c.copyWireStats(&result.Metrics, stats)
		return result, err
	}
	resources, baseComplete, baseErr := c.fetchBaseResources(ctx, root, stats)
	graph, graphErr := c.collectResourceGraph(ctx, root, resources, stats)
	collectionErr := errors.Join(baseErr, graphErr)
	result.Complete = baseComplete && baseErr == nil && graph.Complete && graphErr == nil
	if identityIntegrityError(graphErr) {
		result.Complete = false
		result.Diagnostics = append(result.Diagnostics, graph.finalDiagnostics()...)
		result.Diagnostics = append(result.Diagnostics, boundedDiagnostic(graphErr.Error()))
		c.finishCollectionResult(&result, graph, stats, started)
		return result, collectionErr
	}
	var hardwareErr error
	result.Hardware, hardwareErr = c.hardwareSurface(graph, result.ObservedAt)
	collectionErr = errors.Join(collectionErr, hardwareErr)
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
		return result, collectionErr
	}
	c.finishCollectionResult(&result, graph, stats, started)
	return result, collectionErr
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
