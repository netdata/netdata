// Package jmx provides a typed adapter around the WebSphere JMX helper bridge.
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package jmx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/jmxbridge"
)

// NewClient constructs a WebSphere JMX protocol client.
func NewClient(cfg Config, logger jmxbridge.Logger, opts ...Option) (*Client, error) {
	if logger == nil {
		return nil, errors.New("websphere jmx protocol: logger is required")
	}

	trimmedURL := strings.TrimSpace(cfg.JMXURL)
	if trimmedURL == "" {
		return nil, errors.New("websphere jmx protocol: jmx_url is required")
	}
	cfg.JMXURL = trimmedURL

	if cfg.InitTimeout <= 0 {
		cfg.InitTimeout = 30 * time.Second
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 5 * time.Second
	}
	if cfg.ShutdownDelay < 0 {
		cfg.ShutdownDelay = 0
	}

	client := &Client{
		cfg:     cfg,
		logger:  logger,
		jarData: helperJar,
		jarName: helperJarName,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.bridge == nil {
		bridgeCfg := jmxbridge.Config{
			JavaExecPath: cfg.JavaExecPath,
			JarData:      client.jarData,
			JarFileName:  client.jarName,
		}
		bridge, err := jmxbridge.NewClient(bridgeCfg, logger)
		if err != nil {
			return nil, fmt.Errorf("websphere jmx protocol: creating bridge failed: %w", err)
		}
		client.bridge = bridge
	}

	return client, nil
}

// Start launches the helper process and performs the INIT handshake.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	startCtx, cancel := context.WithTimeout(ctx, c.cfg.InitTimeout)
	defer cancel()

	cmd := jmxbridge.Command{
		"command":          "INIT",
		"protocol_version": protocolVersion,
		"jmx_url":          c.cfg.JMXURL,
	}
	if c.cfg.JMXUsername != "" {
		cmd["jmx_username"] = c.cfg.JMXUsername
	}
	if c.cfg.JMXPassword != "" {
		cmd["jmx_password"] = c.cfg.JMXPassword
	}
	if c.cfg.JMXClasspath != "" {
		cmd["jmx_classpath"] = c.cfg.JMXClasspath
	}

	if err := c.bridge.Start(startCtx, cmd); err != nil {
		return fmt.Errorf("websphere jmx protocol: helper init failed: %w", err)
	}

	c.started = true
	return nil
}

// Shutdown stops the helper process.
func (c *Client) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}

	if c.cfg.ShutdownDelay > 0 {
		time.Sleep(c.cfg.ShutdownDelay)
	}

	c.bridge.Shutdown()
	c.started = false
}

// FetchJVM requests JVM metrics from the helper.
func (c *Client) FetchJVM(ctx context.Context) (*JVMStats, error) {
	payload, err := c.send(ctx, jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "JVM",
	})
	if err != nil {
		return nil, err
	}

	stats := &JVMStats{}

	heap := mapValue(payload, "heap")
	stats.Heap.Used = floatValue(heap, "used")
	stats.Heap.Committed = floatValue(heap, "committed")
	stats.Heap.Max = floatValue(heap, "max")

	nonheap := mapValue(payload, "nonheap")
	stats.NonHeap.Used = floatValue(nonheap, "used")
	stats.NonHeap.Committed = floatValue(nonheap, "committed")

	gc := mapValue(payload, "gc")
	stats.GC.Count = floatValue(gc, "count")
	stats.GC.Time = floatValue(gc, "time")

	threads := mapValue(payload, "threads")
	stats.Threads.Count = floatValue(threads, "count")
	stats.Threads.Daemon = floatValue(threads, "daemon")
	stats.Threads.Peak = floatValue(threads, "peak")
	stats.Threads.Started = floatValue(threads, "totalStarted")

	classes := mapValue(payload, "classes")
	stats.Classes.Loaded = floatValue(classes, "loaded")
	stats.Classes.Unloaded = floatValue(classes, "unloaded")

	cpu := mapValue(payload, "cpu")
	stats.CPU.ProcessUsage = floatValue(cpu, "processCpuUsage")

	stats.Uptime = floatValue(payload, "uptime")

	return stats, nil
}

// FetchThreadPools retrieves thread pool metrics from the helper.
func (c *Client) FetchThreadPools(ctx context.Context, maxItems int) ([]ThreadPool, error) {
	cmd := jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "THREADPOOLS",
	}
	if maxItems > 0 {
		cmd["max_items"] = maxItems
	}

	payload, err := c.send(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var pools []ThreadPool
	items, ok := payload["threadPools"].([]interface{})
	if !ok {
		return pools, nil
	}

	for _, item := range items {
		poolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(poolMap, "name")
		if name == "" {
			continue
		}

		pools = append(pools, ThreadPool{
			Name:            name,
			PoolSize:        floatValue(poolMap, "poolSize"),
			ActiveCount:     floatValue(poolMap, "activeCount"),
			MaximumPoolSize: floatValue(poolMap, "maximumPoolSize"),
		})
	}

	return pools, nil
}

// FetchJDBCPools retrieves JDBC pool statistics from the helper.
func (c *Client) FetchJDBCPools(ctx context.Context, maxItems int) ([]JDBCPool, error) {
	cmd := jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "JDBC",
	}
	if maxItems > 0 {
		cmd["max_items"] = maxItems
	}

	payload, err := c.send(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var pools []JDBCPool
	items, ok := payload["jdbcPools"].([]interface{})
	if !ok {
		return pools, nil
	}

	for _, item := range items {
		poolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(poolMap, "name")
		if name == "" {
			continue
		}

		pools = append(pools, JDBCPool{
			Name:                    name,
			PoolSize:                floatValue(poolMap, "poolSize"),
			NumConnectionsUsed:      floatValue(poolMap, "numConnectionsUsed"),
			NumConnectionsFree:      floatValue(poolMap, "numConnectionsFree"),
			AvgWaitTime:             floatValue(poolMap, "avgWaitTime"),
			AvgInUseTime:            floatValue(poolMap, "avgInUseTime"),
			NumConnectionsCreated:   floatValue(poolMap, "numConnectionsCreated"),
			NumConnectionsDestroyed: floatValue(poolMap, "numConnectionsDestroyed"),
			WaitingThreadCount:      floatValue(poolMap, "waitingThreadCount"),
		})
	}

	return pools, nil
}

// FetchJCAPools retrieves JCA pool statistics from the helper.
func (c *Client) FetchJCAPools(ctx context.Context, maxItems int) ([]JCAPool, error) {
	cmd := jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "JCA",
	}
	if maxItems > 0 {
		cmd["max_items"] = maxItems
	}

	payload, err := c.send(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var pools []JCAPool
	items, ok := payload["jcaPools"].([]interface{})
	if !ok {
		return pools, nil
	}

	for _, item := range items {
		poolMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(poolMap, "name")
		if name == "" {
			continue
		}

		pools = append(pools, JCAPool{
			Name:                    name,
			PoolSize:                floatValue(poolMap, "poolSize"),
			NumConnectionsUsed:      floatValue(poolMap, "numConnectionsUsed"),
			NumConnectionsFree:      floatValue(poolMap, "numConnectionsFree"),
			AvgWaitTime:             floatValue(poolMap, "avgWaitTime"),
			AvgInUseTime:            floatValue(poolMap, "avgInUseTime"),
			NumConnectionsCreated:   floatValue(poolMap, "numConnectionsCreated"),
			NumConnectionsDestroyed: floatValue(poolMap, "numConnectionsDestroyed"),
			WaitingThreadCount:      floatValue(poolMap, "waitingThreadCount"),
		})
	}

	return pools, nil
}

// FetchJMSDestinations retrieves JMS metrics from the helper.
func (c *Client) FetchJMSDestinations(ctx context.Context, maxItems int) ([]JMSDestination, error) {
	cmd := jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "JMS",
	}
	if maxItems > 0 {
		cmd["max_items"] = maxItems
	}

	payload, err := c.send(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var dests []JMSDestination
	items, ok := payload["jmsDestinations"].([]interface{})
	if !ok {
		return dests, nil
	}

	for _, item := range items {
		destMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(destMap, "name")
		if name == "" {
			continue
		}

		dests = append(dests, JMSDestination{
			Name:                 name,
			Type:                 stringValue(destMap, "type"),
			MessagesCurrentCount: floatValue(destMap, "messagesCurrentCount"),
			MessagesPendingCount: floatValue(destMap, "messagesPendingCount"),
			MessagesAddedCount:   floatValue(destMap, "messagesAddedCount"),
			ConsumerCount:        floatValue(destMap, "consumerCount"),
		})
	}

	return dests, nil
}

// FetchApplications retrieves web application metrics from the helper.
func (c *Client) FetchApplications(ctx context.Context, maxItems int, includeSessions, includeTransactions bool) ([]ApplicationMetric, error) {
	cmd := jmxbridge.Command{
		"command": "SCRAPE",
		"target":  "APPLICATIONS",
	}
	if maxItems > 0 {
		cmd["max_items"] = maxItems
	}
	cmd["collect_options"] = map[string]bool{
		"sessions":     includeSessions,
		"transactions": includeTransactions,
	}

	payload, err := c.send(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var metrics []ApplicationMetric
	items, ok := payload["applications"].([]interface{})
	if !ok {
		return metrics, nil
	}

	for _, item := range items {
		appMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(appMap, "name")
		if name == "" {
			continue
		}

		metric := ApplicationMetric{
			Name:                   name,
			Module:                 stringValue(appMap, "module"),
			Requests:               floatValue(appMap, "requestCount"),
			ResponseTime:           floatValue(appMap, "averageResponseTime"),
			ActiveSessions:         floatValue(appMap, "activeSessions"),
			LiveSessions:           floatValue(appMap, "liveSessions"),
			SessionCreates:         floatValue(appMap, "sessionCreates"),
			SessionInvalidates:     floatValue(appMap, "sessionInvalidates"),
			TransactionsCommitted:  floatValue(appMap, "transactionsCommitted"),
			TransactionsRolledback: floatValue(appMap, "transactionsRolledBack"),
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

func (c *Client) send(ctx context.Context, cmd jmxbridge.Command) (map[string]interface{}, error) {
	if !c.started {
		return nil, errors.New("websphere jmx protocol: client not started")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, c.cfg.CommandTimeout)
	defer cancel()

	resp, err := c.bridge.Send(cmdCtx, cmd)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("websphere jmx protocol: command failed: %s", resp.Message)
		}
		return nil, fmt.Errorf("websphere jmx protocol: command failed: %w", err)
	}

	if resp == nil {
		return nil, errors.New("websphere jmx protocol: empty response")
	}

	return resp.Data, nil
}

func mapValue(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	val, _ := m[key].(map[string]interface{})
	if val == nil {
		return map[string]interface{}{}
	}
	return val
}

func stringValue(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatValue(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	return toFloat(m[key])
}

func toFloat(v interface{}) float64 {
	switch value := v.(type) {
	case nil:
		return 0
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case uint:
		return float64(value)
	case uint64:
		return float64(value)
	case uint32:
		return float64(value)
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
		return 0
	case json.Number:
		parsed, err := value.Float64()
		if err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}
