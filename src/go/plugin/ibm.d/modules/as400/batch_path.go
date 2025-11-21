//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

const (
	queryNameMessageQueueTotals = "message_queue_totals"
	queryNameJobQueueTotals     = "job_queue_totals"
	queryNameOutputQueueTotals  = "output_queue_totals"
)

type batchPathConfig struct {
	enabled        bool
	interval       time.Duration
	maxConnections int
}

type queueTotalsSnapshot struct {
	timestamp time.Time
	err       error
	queues    map[string]int64
	items     map[string]int64
}

type batchCache struct {
	mu      sync.RWMutex
	totals  queueTotalsSnapshot
	latency latencyCache
}

func (c *Collector) batchTotalsEnabled() bool {
	return c != nil && (c.CollectMessageQueueTotals.IsEnabled() || c.CollectJobQueueTotals.IsEnabled() || c.CollectOutputQueueTotals.IsEnabled())
}

func (c *Collector) startBatchPath() error {
	c.stopBatchPath()

	totalsEnabled := c.batchTotalsEnabled()
	if !c.BatchPath && totalsEnabled {
		c.Infof("batch path not started: batch_path is disabled while totals are enabled (message=%s job=%s output=%s)",
			c.CollectMessageQueueTotals.String(), c.CollectJobQueueTotals.String(), c.CollectOutputQueueTotals.String())
	}

	cfg := batchPathConfig{
		enabled:        c.BatchPath && totalsEnabled,
		interval:       time.Duration(c.BatchPathUpdateEvery),
		maxConnections: c.BatchPathMaxConnections,
	}

	if cfg.interval <= 0 {
		cfg.interval = time.Minute
	}
	if cfg.interval < time.Minute {
		c.Warningf("batch path update every %s is shorter than 1m; using 1m", cfg.interval)
		cfg.interval = time.Minute
	}
	if cfg.maxConnections <= 0 {
		cfg.maxConnections = 1
	}

	c.batch.config = cfg

	if !cfg.enabled {
		c.Infof("batch path not started: totals-enabled=%t (message=%s job=%s output=%s) batch_path=%t",
			totalsEnabled,
			c.CollectMessageQueueTotals.String(), c.CollectJobQueueTotals.String(), c.CollectOutputQueueTotals.String(),
			c.BatchPath)
		return nil
	}

	clientCfg := as400proto.Config{
		DSN:          c.DSN,
		Timeout:      time.Duration(c.Timeout),
		MaxOpenConns: cfg.maxConnections,
	}

	client := as400proto.NewClient(clientCfg)
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("batch path: connect failed: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("batch path: ping failed: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	c.batch.client = client
	c.batch.cancel = cancel
	c.batch.wg.Add(1)
	go c.runBatchPath(runCtx)
	c.Infof("batch path worker started (interval=%s, max_conns=%d)", cfg.interval, cfg.maxConnections)
	return nil
}

func (c *Collector) stopBatchPath() {
	if c.batch.cancel != nil {
		c.batch.cancel()
	}
	c.batch.wg.Wait()
	if c.batch.client != nil {
		if err := c.batch.client.Close(); err != nil {
			c.Errorf("batch path: closing client failed: %v", err)
		}
	}
	c.batch.cancel = nil
	c.batch.client = nil
	c.batch.config = batchPathConfig{}
}

func (c *Collector) runBatchPath(ctx context.Context) {
	defer c.batch.wg.Done()

	interval := c.batch.config.interval
	if interval <= 0 {
		interval = time.Minute
	}

	now := time.Now()
	beat := now
	c.runBatchCollectors(ctx, beat)
	nextBeat := beat.Add(interval)

	for {
		sleep := time.Until(nextBeat)
		if sleep > 0 {
			timer := time.NewTimer(sleep)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		beat = nextBeat
		c.runBatchCollectors(ctx, beat)

		nextBeat = nextBeat.Add(interval)
		now = time.Now()
		for nextBeat.Before(now) {
			nextBeat = nextBeat.Add(interval)
		}
	}
}

func (c *Collector) runBatchCollectors(ctx context.Context, beat time.Time) {
	if ctx.Err() != nil {
		return
	}

	c.batch.cache.beginLatencyCycle(beat)

	if !c.batchTotalsEnabled() {
		c.batch.cache.setTotals(queueTotalsSnapshot{timestamp: beat})
		return
	}

	snapshot, err := c.fetchQueueTotals(ctx, beat, c.batchDoQueryRow)
	c.batch.cache.setTotals(snapshot)
	if err != nil && !errors.Is(err, context.Canceled) {
		c.logErrorOnce("batch_path_error", "batch path: %s", trimDriverMessage(err))
	} else if err == nil {
		c.clearErrorOnce("batch_path_error")
	}
}

func (c *Collector) fetchQueueTotals(ctx context.Context, beat time.Time, do queryRowFunc) (queueTotalsSnapshot, error) {
	snapshot := queueTotalsSnapshot{
		timestamp: beat,
		queues:    make(map[string]int64),
		items:     make(map[string]int64),
	}

	var firstErr error

	if c.CollectMessageQueueTotals.IsEnabled() {
		var messageCount, queueCount int64
		err := do(ctx, queryNameMessageQueueTotals, queryMessageQueueTotals, func(column, value string) {
			switch column {
			case "MESSAGE_COUNT":
				messageCount = parseInt64OrZero(value)
			case "QUEUE_COUNT":
				queueCount = parseInt64OrZero(value)
			}
		})
		if err != nil {
			c.logQueryErrorOnce("batch_message_queue_totals", queryMessageQueueTotals, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("message queue totals: %w", err)
			}
		} else {
			c.clearErrorOnce("batch_message_queue_totals")
			snapshot.queues["message_queue"] = queueCount
			snapshot.items["message_queue"] = messageCount
		}
	}

	if c.CollectJobQueueTotals.IsEnabled() {
		var queueCount, jobCount int64
		err := do(ctx, queryNameJobQueueTotals, queryJobQueueTotals, func(column, value string) {
			switch column {
			case "QUEUE_COUNT":
				queueCount = parseInt64OrZero(value)
			case "JOB_COUNT":
				jobCount = parseInt64OrZero(value)
			}
		})
		if err != nil {
			c.logQueryErrorOnce("batch_job_queue_totals", queryJobQueueTotals, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("job queue totals: %w", err)
			}
		} else {
			c.clearErrorOnce("batch_job_queue_totals")
			snapshot.queues["job_queue"] = queueCount
			snapshot.items["job_queue"] = jobCount
		}
	}

	if c.CollectOutputQueueTotals.IsEnabled() {
		var queueCount, fileCount int64
		err := do(ctx, queryNameOutputQueueTotals, queryOutputQueueTotals, func(column, value string) {
			switch column {
			case "QUEUE_COUNT":
				queueCount = parseInt64OrZero(value)
			case "FILE_COUNT":
				fileCount = parseInt64OrZero(value)
			}
		})
		if err != nil {
			c.logQueryErrorOnce("batch_output_queue_totals", queryOutputQueueTotals, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("output queue totals: %w", err)
			}
		} else {
			c.clearErrorOnce("batch_output_queue_totals")
			snapshot.queues["output_queue"] = queueCount
			snapshot.items["output_queue"] = fileCount
		}
	}

	snapshot.err = firstErr
	return snapshot, snapshot.err
}

func (c *Collector) batchDoQueryRow(ctx context.Context, queryName, query string, assign func(column, value string)) error {
	if c.batch.client == nil {
		return errors.New("batch path client not initialised")
	}

	start := time.Now()
	err := c.queryRowWithClient(ctx, c.batch.client, queryName, query, assign)
	elapsed := time.Since(start)
	latency := elapsed.Microseconds()
	if latency == 0 {
		latency = 1
	}
	c.batch.cache.addLatency(queryName, latency)
	c.Debugf("batch recorded %s=%dµs", queryName, latency)
	return err
}

func (c *Collector) batchPathActive() bool {
	return c != nil && c.batch.config.enabled && c.batch.client != nil
}

func (c *Collector) batchPathIntervalSeconds() int {
	if c == nil {
		return 0
	}
	if !c.batchPathActive() {
		return 0
	}
	interval := int(c.batch.config.interval / time.Second)
	if interval < 1 {
		interval = 60
	}
	return interval
}

func (c *batchCache) beginLatencyCycle(ts time.Time) {
	c.latency.beginCycle(ts)
}

func (c *batchCache) addLatency(name string, value int64) {
	c.latency.add(name, value)
}

func (c *batchCache) setTotals(snapshot queueTotalsSnapshot) {
	c.mu.Lock()
	c.totals = snapshot
	c.mu.Unlock()
}

func (c *batchCache) getTotals() queueTotalsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneQueueTotalsSnapshot(c.totals)
}

func (c *batchCache) getLatencies() (map[string]int64, time.Time) {
	return c.latency.snapshot()
}

func cloneQueueTotalsSnapshot(src queueTotalsSnapshot) queueTotalsSnapshot {
	dst := queueTotalsSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.queues != nil {
		dst.queues = make(map[string]int64, len(src.queues))
		for k, v := range src.queues {
			dst.queues[k] = v
		}
	}
	if src.items != nil {
		dst.items = make(map[string]int64, len(src.items))
		for k, v := range src.items {
			dst.items[k] = v
		}
	}
	return dst
}
