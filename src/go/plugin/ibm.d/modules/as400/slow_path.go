//go:build cgo
// +build cgo

package as400

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

type slowPathConfig struct {
	enabled        bool
	interval       time.Duration
	maxConnections int
}

type messageQueueSnapshot struct {
	metrics   map[string]messageQueueInstanceMetrics
	meta      map[string]messageQueueMetrics
	timestamp time.Time
	err       error
}

type jobQueueSnapshot struct {
	metrics   map[string]jobQueueInstanceMetrics
	meta      map[string]jobQueueMetrics
	timestamp time.Time
	err       error
}

type outputQueueSnapshot struct {
	metrics   map[string]outputQueueInstanceMetrics
	meta      map[string]outputQueueMetrics
	timestamp time.Time
	err       error
}

type subsystemSnapshot struct {
	metrics   map[string]subsystemInstanceMetrics
	meta      map[string]subsystemMetrics
	timestamp time.Time
	err       error
}

type planCacheSnapshot struct {
	values    map[string]planCacheInstanceMetrics
	meta      map[string]planCacheMetrics
	timestamp time.Time
	err       error
}

type slowCache struct {
	mu            sync.RWMutex
	messageQueues messageQueueSnapshot
	jobQueues     jobQueueSnapshot
	outputQueues  outputQueueSnapshot
	subsystems    subsystemSnapshot
	planCache     planCacheSnapshot
	latencies     map[string]int64
	lastLatencyTs time.Time
}

func (c *Collector) slowPathActive() bool {
	return c != nil && c.slow.config.enabled && c.slow.client != nil
}

func (c *slowCache) beginLatencyCycle(ts time.Time) {
	c.mu.Lock()
	if c.latencies == nil {
		c.latencies = make(map[string]int64)
	}
	c.lastLatencyTs = ts
	c.mu.Unlock()
}

func (c *slowCache) addLatency(name string, value int64) {
	if value == 0 {
		return
	}
	c.mu.Lock()
	if c.latencies == nil {
		c.latencies = make(map[string]int64)
	}
	c.latencies[name] += value
	c.mu.Unlock()
}

func (c *slowCache) setMessageQueues(snapshot messageQueueSnapshot) {
	c.mu.Lock()
	c.messageQueues = snapshot
	c.mu.Unlock()
}

func (c *slowCache) setJobQueues(snapshot jobQueueSnapshot) {
	c.mu.Lock()
	c.jobQueues = snapshot
	c.mu.Unlock()
}

func (c *slowCache) setOutputQueues(snapshot outputQueueSnapshot) {
	c.mu.Lock()
	c.outputQueues = snapshot
	c.mu.Unlock()
}

func (c *slowCache) setSubsystems(snapshot subsystemSnapshot) {
	c.mu.Lock()
	c.subsystems = snapshot
	c.mu.Unlock()
}

func (c *slowCache) setPlanCache(snapshot planCacheSnapshot) {
	c.mu.Lock()
	c.planCache = snapshot
	c.mu.Unlock()
}

func (c *slowCache) getMessageQueues() messageQueueSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneMessageQueueSnapshot(c.messageQueues)
}

func (c *slowCache) getJobQueues() jobQueueSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneJobQueueSnapshot(c.jobQueues)
}

func (c *slowCache) getOutputQueues() outputQueueSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneOutputQueueSnapshot(c.outputQueues)
}

func (c *slowCache) getSubsystems() subsystemSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSubsystemSnapshot(c.subsystems)
}

func (c *slowCache) getPlanCache() planCacheSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return clonePlanCacheSnapshot(c.planCache)
}

func (c *slowCache) getLatencies() (map[string]int64, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.latencies) == 0 {
		return nil, c.lastLatencyTs
	}
	copyMap := make(map[string]int64, len(c.latencies))
	for k, v := range c.latencies {
		copyMap[k] = v
	}
	return copyMap, c.lastLatencyTs
}

func cloneMessageQueueSnapshot(src messageQueueSnapshot) messageQueueSnapshot {
	dst := messageQueueSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.metrics != nil {
		dst.metrics = make(map[string]messageQueueInstanceMetrics, len(src.metrics))
		for k, v := range src.metrics {
			dst.metrics[k] = v
		}
	}
	if src.meta != nil {
		dst.meta = make(map[string]messageQueueMetrics, len(src.meta))
		for k, v := range src.meta {
			dst.meta[k] = v
		}
	}
	return dst
}

func cloneJobQueueSnapshot(src jobQueueSnapshot) jobQueueSnapshot {
	dst := jobQueueSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.metrics != nil {
		dst.metrics = make(map[string]jobQueueInstanceMetrics, len(src.metrics))
		for k, v := range src.metrics {
			dst.metrics[k] = v
		}
	}
	if src.meta != nil {
		dst.meta = make(map[string]jobQueueMetrics, len(src.meta))
		for k, v := range src.meta {
			dst.meta[k] = v
		}
	}
	return dst
}

func cloneOutputQueueSnapshot(src outputQueueSnapshot) outputQueueSnapshot {
	dst := outputQueueSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.metrics != nil {
		dst.metrics = make(map[string]outputQueueInstanceMetrics, len(src.metrics))
		for k, v := range src.metrics {
			dst.metrics[k] = v
		}
	}
	if src.meta != nil {
		dst.meta = make(map[string]outputQueueMetrics, len(src.meta))
		for k, v := range src.meta {
			dst.meta[k] = v
		}
	}
	return dst
}

func cloneSubsystemSnapshot(src subsystemSnapshot) subsystemSnapshot {
	dst := subsystemSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.metrics != nil {
		dst.metrics = make(map[string]subsystemInstanceMetrics, len(src.metrics))
		for k, v := range src.metrics {
			dst.metrics[k] = v
		}
	}
	if src.meta != nil {
		dst.meta = make(map[string]subsystemMetrics, len(src.meta))
		for k, v := range src.meta {
			dst.meta[k] = v
		}
	}
	return dst
}

func clonePlanCacheSnapshot(src planCacheSnapshot) planCacheSnapshot {
	dst := planCacheSnapshot{
		timestamp: src.timestamp,
		err:       src.err,
	}
	if src.values != nil {
		dst.values = make(map[string]planCacheInstanceMetrics, len(src.values))
		for k, v := range src.values {
			dst.values[k] = v
		}
	}
	if src.meta != nil {
		dst.meta = make(map[string]planCacheMetrics, len(src.meta))
		for k, v := range src.meta {
			dst.meta[k] = v
		}
	}
	return dst
}

func (c *Collector) startSlowPath() error {
	c.stopSlowPath()

	cfg := slowPathConfig{
		enabled:        c.SlowPath,
		interval:       time.Duration(c.SlowPathUpdateEvery),
		maxConnections: c.SlowPathMaxConnections,
	}

	if !cfg.enabled {
		c.Debugf("slow path disabled; running sequential-only mode")
		c.slow.config = cfg
		return nil
	}

	if cfg.interval <= 0 {
		cfg.interval = 30 * time.Second
	}
	if cfg.maxConnections <= 0 {
		cfg.maxConnections = 1
	}

	fastInterval := time.Duration(c.fastPathIntervalSeconds()) * time.Second
	if fastInterval <= 0 {
		fastInterval = time.Second
	}
	if cfg.interval < fastInterval {
		c.Warningf("slow path update every %s is shorter than main update %s; using %s", cfg.interval, fastInterval, fastInterval)
		cfg.interval = fastInterval
	}

	clientCfg := as400proto.Config{
		DSN:          c.DSN,
		Timeout:      time.Duration(c.Timeout),
		MaxOpenConns: cfg.maxConnections,
	}

	client := as400proto.NewClient(clientCfg)
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("slow path: connect failed: %w", err)
	}
	if err := client.Ping(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("slow path: ping failed: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	c.slow.client = client
	c.slow.cancel = cancel
	c.slow.config = cfg
	c.slow.wg.Add(1)
	go c.runSlowPath(runCtx)
	c.Infof("slow path worker started (interval=%s, max_conns=%d)", cfg.interval, cfg.maxConnections)
	return nil
}

func (c *Collector) stopSlowPath() {
	if c.slow.cancel != nil {
		c.slow.cancel()
	}
	c.slow.wg.Wait()
	if c.slow.client != nil {
		if err := c.slow.client.Close(); err != nil {
			c.Errorf("slow path: closing client failed: %v", err)
		}
	}
	c.slow.cancel = nil
	c.slow.client = nil
	c.slow.config = slowPathConfig{}
	c.slow.cache = slowCache{}
}

func (c *Collector) runSlowPath(ctx context.Context) {
	defer c.slow.wg.Done()

	interval := c.slow.config.interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	now := time.Now()
	beat := now
	c.runSlowCollectors(ctx, beat)
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
		c.runSlowCollectors(ctx, beat)

		nextBeat = nextBeat.Add(interval)
		now = time.Now()
		for nextBeat.Before(now) {
			nextBeat = nextBeat.Add(interval)
		}
	}
}

func (c *Collector) runSlowCollectors(ctx context.Context, beat time.Time) {
	if ctx.Err() != nil {
		return
	}

	c.slow.cache.beginLatencyCycle(beat)

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	group, groupCtx := errgroup.WithContext(workCtx)
	group.SetLimit(c.slow.config.maxConnections)

	group.Go(func() error {
		snapshot, err := c.fetchMessageQueues(groupCtx, beat, c.slowDoQuery)
		c.slow.cache.setMessageQueues(snapshot)
		if err != nil {
			return fmt.Errorf("message queues: %w", err)
		}
		return nil
	})

	group.Go(func() error {
		snapshot, err := c.fetchJobQueues(groupCtx, beat, c.slowDoQuery)
		c.slow.cache.setJobQueues(snapshot)
		if err != nil {
			return fmt.Errorf("job queues: %w", err)
		}
		return nil
	})

	group.Go(func() error {
		snapshot, err := c.fetchOutputQueues(groupCtx, beat, c.slowDoQuery)
		c.slow.cache.setOutputQueues(snapshot)
		if err != nil {
			return fmt.Errorf("output queues: %w", err)
		}
		return nil
	})

	group.Go(func() error {
		snapshot, err := c.fetchSubsystems(groupCtx, beat, c.slowDoQuery, c.slowDoQueryRow)
		c.slow.cache.setSubsystems(snapshot)
		if err != nil {
			return fmt.Errorf("subsystems: %w", err)
		}
		return nil
	})

	if c.CollectPlanCacheMetrics.IsEnabled() {
		group.Go(func() error {
			snapshot, err := c.fetchPlanCache(groupCtx, beat, c.slowExec, c.slowDoQuery)
			c.slow.cache.setPlanCache(snapshot)
			if err != nil {
				return fmt.Errorf("plan cache: %w", err)
			}
			return nil
		})
	} else {
		c.slow.cache.setPlanCache(planCacheSnapshot{
			timestamp: beat,
			err:       nil,
			values:    make(map[string]planCacheInstanceMetrics),
			meta:      make(map[string]planCacheMetrics),
		})
	}

	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		c.logErrorOnce("slow_path_error", "slow path: %s", trimDriverMessage(err))
	} else if err == nil {
		c.clearErrorOnce("slow_path_error")
	}
}

type queryFunc func(ctx context.Context, queryName, query string, assign func(column, value string, lineEnd bool)) error
type queryRowFunc func(ctx context.Context, queryName, query string, assign func(column, value string)) error
type execFunc func(ctx context.Context, query string) error

func (c *Collector) fetchMessageQueues(ctx context.Context, beat time.Time, do queryFunc) (messageQueueSnapshot, error) {
	snapshot := messageQueueSnapshot{
		metrics:   make(map[string]messageQueueInstanceMetrics),
		meta:      make(map[string]messageQueueMetrics),
		timestamp: beat,
	}

	if len(c.messageQueueTargets) == 0 {
		return snapshot, nil
	}

	var firstErr error

	for _, target := range c.messageQueueTargets {
		key := target.ID()
		errorKey := "slow_message_queue_" + key
		meta := messageQueueMetrics{
			library: target.Library,
			name:    target.Name,
		}
		metrics := messageQueueInstanceMetrics{}

		queryName := fmt.Sprintf("message_queue_%s_%s", target.Library, target.Name)
		query := buildMessageQueueQuery(target, c.supportsMessageQueueTableFunction())
		err := do(ctx, queryName, query, func(column, value string, lineEnd bool) {
			switch column {
			case "MESSAGE_COUNT":
				metrics.Total = parseInt64OrZero(value)
			case "INFORMATIONAL_MESSAGES":
				metrics.Informational = parseInt64OrZero(value)
			case "INQUIRY_MESSAGES":
				metrics.Inquiry = parseInt64OrZero(value)
			case "DIAGNOSTIC_MESSAGES":
				metrics.Diagnostic = parseInt64OrZero(value)
			case "ESCAPE_MESSAGES":
				metrics.Escape = parseInt64OrZero(value)
			case "NOTIFY_MESSAGES":
				metrics.Notify = parseInt64OrZero(value)
			case "SENDER_COPY_MESSAGES":
				metrics.SenderCopy = parseInt64OrZero(value)
			case "MAX_SEVERITY":
				metrics.MaxSeverity = parseInt64OrZero(value)
			}
		})

		if err != nil {
			c.logQueryErrorOnce(errorKey, query, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("message queue %s: %w", key, err)
			}
			continue
		}
		c.clearErrorOnce(errorKey)

		snapshot.metrics[key] = metrics
		snapshot.meta[key] = meta
	}

	snapshot.err = firstErr
	return snapshot, firstErr
}

func (c *Collector) fetchJobQueues(ctx context.Context, beat time.Time, do queryFunc) (jobQueueSnapshot, error) {
	snapshot := jobQueueSnapshot{
		metrics:   make(map[string]jobQueueInstanceMetrics),
		meta:      make(map[string]jobQueueMetrics),
		timestamp: beat,
	}

	if len(c.jobQueueTargets) == 0 {
		return snapshot, nil
	}

	var firstErr error

	for _, target := range c.jobQueueTargets {
		key := target.ID()
		errorKey := "slow_job_queue_" + key
		meta := jobQueueMetrics{
			library: target.Library,
			name:    target.Name,
			status:  "UNKNOWN",
		}
		metrics := jobQueueInstanceMetrics{}
		found := false

		queryName := fmt.Sprintf("job_queue_%s_%s", target.Library, target.Name)
		query := buildJobQueueQuery(target)
		err := do(ctx, queryName, query, func(column, value string, lineEnd bool) {
			switch column {
			case "JOB_QUEUE_STATUS":
				meta.status = strings.TrimSpace(value)
			case "NUMBER_OF_JOBS":
				metrics.NumberOfJobs = parseInt64OrZero(value)
			case "RELEASED_JOBS":
				meta.jobsWaiting = parseInt64OrZero(value)
			case "SCHEDULED_JOBS":
				meta.jobsScheduled = parseInt64OrZero(value)
			case "HELD_JOBS":
				meta.jobsHeld = parseInt64OrZero(value)
			case "MAXIMUM_ACTIVE_JOBS":
				meta.maxJobs = parseInt64OrZero(value)
			}
			if lineEnd {
				found = true
			}
		})

		if err != nil {
			c.logQueryErrorOnce(errorKey, query, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("job queue %s: %w", key, err)
			}
			continue
		}
		c.clearErrorOnce(errorKey)

		if !found {
			meta.status = "NOT_FOUND"
		}

		snapshot.metrics[key] = metrics
		snapshot.meta[key] = meta
	}

	snapshot.err = firstErr
	return snapshot, firstErr
}

func (c *Collector) fetchOutputQueues(ctx context.Context, beat time.Time, do queryFunc) (outputQueueSnapshot, error) {
	snapshot := outputQueueSnapshot{
		metrics:   make(map[string]outputQueueInstanceMetrics),
		meta:      make(map[string]outputQueueMetrics),
		timestamp: beat,
	}

	if len(c.outputQueueTargets) == 0 {
		return snapshot, nil
	}

	var firstErr error

	for _, target := range c.outputQueueTargets {
		key := target.ID()
		errorEntriesKey := "slow_output_queue_entries_" + key
		errorInfoKey := "slow_output_queue_info_" + key
		meta := outputQueueMetrics{
			library: target.Library,
			name:    target.Name,
			status:  "UNKNOWN",
		}

		metrics := outputQueueInstanceMetrics{}
		entriesCount := int64(0)
		entriesUsed := false

		queryName := fmt.Sprintf("output_queue_%s_%s", target.Library, target.Name)
		entriesQuery := buildOutputQueueEntriesQuery(target)
		err := do(ctx, queryName, entriesQuery, func(column, value string, lineEnd bool) {
			if lineEnd {
				entriesCount++
			}
		})
		if err != nil {
			c.logQueryErrorOnce(errorEntriesKey, entriesQuery, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("output queue %s (entries): %w", key, err)
			}
		} else {
			c.clearErrorOnce(errorEntriesKey)
			entriesUsed = true
			metrics.Files = entriesCount
		}

		infoQuery := buildOutputQueueInfoQuery(target)
		viewErr := do(ctx, queryName+"_view", infoQuery, func(column, value string, lineEnd bool) {
			switch column {
			case "OUTPUT_QUEUE_STATUS":
				meta.status = strings.TrimSpace(value)
			case "NUMBER_OF_WRITERS":
				metrics.Writers = parseInt64OrZero(value)
			case "NUMBER_OF_FILES":
				if !entriesUsed {
					metrics.Files = parseInt64OrZero(value)
				}
			}
		})
		if viewErr != nil {
			c.logQueryErrorOnce(errorInfoKey, infoQuery, viewErr)
			if firstErr == nil {
				firstErr = fmt.Errorf("output queue %s (info): %w", key, viewErr)
			}
			continue
		}
		c.clearErrorOnce(errorInfoKey)

		metrics.Released = boolToInt(strings.EqualFold(meta.status, "RELEASED"))
		snapshot.metrics[key] = metrics
		snapshot.meta[key] = meta
	}

	snapshot.err = firstErr
	return snapshot, firstErr
}

func (c *Collector) countSubsystemsWith(doRow queryRowFunc, ctx context.Context) (int, error) {
	var count int64
	err := doRow(ctx, "count_subsystems", queryCountSubsystems, func(column, value string) {
		if column == "COUNT" {
			if v, ok := c.parseInt64Value(value, 1); ok {
				count = v
			}
		}
	})
	return int(count), err
}

func (c *Collector) fetchSubsystems(ctx context.Context, beat time.Time, do queryFunc, doRow queryRowFunc) (subsystemSnapshot, error) {
	snapshot := subsystemSnapshot{
		metrics:   make(map[string]subsystemInstanceMetrics),
		meta:      make(map[string]subsystemMetrics),
		timestamp: beat,
	}

	query := querySubsystems
	if c.MaxSubsystems > 0 {
		if total, err := c.countSubsystemsWith(doRow, ctx); err != nil {
			c.logOnce("subsystem_count_failed", "failed to count subsystems before applying limit: %v", err)
		} else if total > c.MaxSubsystems {
			c.logOnce("subsystem_limit", "subsystem count (%d) exceeds limit (%d); truncating results", total, c.MaxSubsystems)
		}
		query = withFetchLimit(query, c.MaxSubsystems)
	}

	currentSubsystem := ""
	err := do(ctx, "subsystems", query, func(column, value string, lineEnd bool) {
		switch column {
		case "SUBSYSTEM_NAME":
			name := strings.TrimSpace(value)
			if name == "" {
				currentSubsystem = ""
				return
			}
			if c.subsystemSelector != nil && !c.subsystemSelector.MatchString(name) {
				currentSubsystem = ""
				return
			}
			currentSubsystem = name
			subsystem := subsystemMetrics{name: name, status: "ACTIVE"}
			parts := strings.SplitN(name, "/", 2)
			if len(parts) == 2 {
				subsystem.library = parts[0]
				subsystem.name = parts[1]
			}
			snapshot.meta[currentSubsystem] = subsystem
		case "CURRENT_ACTIVE_JOBS":
			if currentSubsystem != "" {
				if v, ok := c.parseInt64Value(value, 1); ok {
					if metrics, exists := snapshot.metrics[currentSubsystem]; exists {
						metrics.CurrentActiveJobs = v
						snapshot.metrics[currentSubsystem] = metrics
					} else {
						snapshot.metrics[currentSubsystem] = subsystemInstanceMetrics{CurrentActiveJobs: v}
					}
				}
			}
		case "MAXIMUM_ACTIVE_JOBS":
			if currentSubsystem != "" {
				if v, ok := c.parseInt64Value(value, 1); ok {
					if metrics, exists := snapshot.metrics[currentSubsystem]; exists {
						metrics.MaximumActiveJobs = v
						snapshot.metrics[currentSubsystem] = metrics
					} else {
						snapshot.metrics[currentSubsystem] = subsystemInstanceMetrics{MaximumActiveJobs: v}
					}
				}
			}
		}

		if lineEnd {
			currentSubsystem = ""
		}
	})

	if err != nil {
		c.logQueryErrorOnce("slow_subsystems", query, err)
		snapshot.err = err
		return snapshot, err
	}
	c.clearErrorOnce("slow_subsystems")
	snapshot.err = nil
	return snapshot, nil
}

func (c *Collector) fetchPlanCache(ctx context.Context, beat time.Time, exec execFunc, do queryFunc) (planCacheSnapshot, error) {
	snapshot := planCacheSnapshot{
		values:    make(map[string]planCacheInstanceMetrics),
		meta:      make(map[string]planCacheMetrics),
		timestamp: beat,
	}

	if err := exec(ctx, callAnalyzePlanCache); err != nil {
		c.logQueryErrorOnce("slow_plan_cache_analyze", callAnalyzePlanCache, err)
		snapshot.err = fmt.Errorf("analyze plan cache: %w", err)
		return snapshot, snapshot.err
	}

	var currentHeading string
	err := do(ctx, "plan_cache_summary", queryPlanCacheSummary, func(column, value string, lineEnd bool) {
		switch column {
		case "HEADING":
			currentHeading = strings.TrimSpace(value)
		case "VALUE":
			if currentHeading == "" {
				return
			}
			key := planCacheMetricKey(currentHeading)
			if key == "" {
				return
			}
			if parsed, ok := c.parseInt64Value(value, precision); ok {
				snapshot.values[key] = planCacheInstanceMetrics{Value: parsed}
				snapshot.meta[key] = planCacheMetrics{heading: currentHeading}
			}
		}
		if lineEnd {
			currentHeading = ""
		}
	})

	if err != nil {
		c.logQueryErrorOnce("slow_plan_cache_summary", queryPlanCacheSummary, err)
		snapshot.err = fmt.Errorf("plan cache summary: %w", err)
		return snapshot, snapshot.err
	}
	c.clearErrorOnce("slow_plan_cache_summary")

	return snapshot, nil
}

func (c *Collector) slowDoQuery(ctx context.Context, queryName, query string, assign func(column, value string, lineEnd bool)) error {
	if c.slow.client == nil {
		return errors.New("slow path client not initialised")
	}

	start := time.Now()
	err := c.queryWithClient(ctx, c.slow.client, queryName, query, assign)
	elapsed := time.Since(start)
	c.slow.cache.addLatency(queryName, elapsed.Microseconds())
	return err
}

func (c *Collector) slowDoQueryRow(ctx context.Context, queryName, query string, assign func(column, value string)) error {
	if c.slow.client == nil {
		return errors.New("slow path client not initialised")
	}

	start := time.Now()
	err := c.queryRowWithClient(ctx, c.slow.client, queryName, query, assign)
	elapsed := time.Since(start)
	c.slow.cache.addLatency(queryName, elapsed.Microseconds())
	return err
}

func (c *Collector) slowExec(ctx context.Context, query string) error {
	if c.slow.client == nil {
		return errors.New("slow path client not initialised")
	}
	start := time.Now()
	err := c.execWithClient(ctx, c.slow.client, query)
	elapsed := time.Since(start)
	c.slow.cache.addLatency("analyze_plan_cache", elapsed.Microseconds())
	return err
}

func (c *Collector) queryWithClient(ctx context.Context, client *as400proto.Client, queryName, query string, assign func(column, value string, lineEnd bool)) error {
	return client.Query(ctx, query, func(columns []string, values []string) error {
		for idx, col := range columns {
			assign(col, values[idx], idx == len(columns)-1)
		}
		return nil
	})
}

func (c *Collector) queryRowWithClient(ctx context.Context, client *as400proto.Client, queryName, query string, assign func(column, value string)) error {
	return client.QueryWithLimit(ctx, query, 1, func(columns []string, values []string) error {
		for idx, col := range columns {
			assign(col, values[idx])
		}
		return nil
	})
}

func (c *Collector) execWithClient(ctx context.Context, client *as400proto.Client, query string) error {
	if err := client.Connect(ctx); err != nil {
		return err
	}
	return client.Exec(ctx, query)
}
