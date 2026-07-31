// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

// Register composes the log-backend collector with the Redfish feature runtime.
func Register(runtime *redfishruntime.Runtime) {
	collectorapi.Register("redfish_logs", newCreator(runtime))
}

func newCreator(runtime *redfishruntime.Runtime) collectorapi.Creator {
	if runtime == nil {
		panic("redfish_logs Register requires a non-nil runtime")
	}
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New(runtime) },
		Config:   func() any { return &Config{} },
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	runtime *redfishruntime.Runtime
	store   metrix.CollectorStore
	metrics *backendMetrics

	jobName  string
	key      string
	dir      string
	maxBytes uint64

	ready                 atomic.Bool
	lastRetentionFailures uint64

	mu           sync.Mutex
	backend      *journalBackend
	registration *redfishruntime.BackendRegistration
	locker       *filelock.Locker
}

// New returns one named Redfish log backend.
func New(runtime *redfishruntime.Runtime) *Collector {
	if runtime == nil {
		panic("redfish_logs New requires a non-nil runtime")
	}
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
		},
		runtime: runtime,
		store:   store,
		metrics: newBackendMetrics(store),
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) SetJobName(name string) { c.jobName = name }

func (c *Collector) Init(context.Context) error {
	c.Config.applyDefaults()
	if err := c.Config.validate(c.jobName); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	maxBytes, err := c.Config.retentionBytes()
	if err != nil {
		return fmt.Errorf("config validation: retention.max_size: %w", err)
	}
	c.maxBytes = maxBytes
	c.key, _ = backendDigest(c.jobName)
	return nil
}

func (c *Collector) Check(context.Context) error {
	info, err := os.Stat(netdataLogDir())
	if err != nil {
		return fmt.Errorf("Redfish log backend: stat Netdata log directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("Redfish log backend: Netdata log path is not a directory")
	}
	return nil
}

func (c *Collector) Run(ctx context.Context) error {
	delay := time.Second
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if err := c.open(); err != nil {
			c.Limit("redfish-logs:backend-open", 1, time.Hour).
				Warningf("Redfish log backend %q is unavailable: %v; retrying", c.jobName, err)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
			delay = min(delay*2, 30*time.Second)
			continue
		}
		break
	}

	<-ctx.Done()
	return nil
}

func (c *Collector) open() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backend != nil {
		return nil
	}

	_, dir, key, err := prepareBackendDirectory(c.jobName)
	if err != nil {
		return err
	}
	locker, err := acquireBackendLock(dir)
	if err != nil {
		return err
	}
	host, err := loadJournalHost()
	if err != nil {
		locker.UnlockAll()
		return err
	}
	backend, err := newJournalBackend(dir, c.maxBytes, host)
	if err != nil {
		locker.UnlockAll()
		return err
	}
	registration, err := c.runtime.RegisterBackend(c.jobName, key, dir, backend)
	if err != nil {
		_ = backend.Close()
		locker.UnlockAll()
		return err
	}

	c.dir = backend.dir
	c.backend = backend
	c.registration = registration
	c.locker = locker
	c.ready.Store(true)
	return nil
}

func (c *Collector) close(ctx context.Context) error {
	c.mu.Lock()
	registration := c.registration
	backend := c.backend
	locker := c.locker
	c.ready.Store(false)
	c.registration = nil
	c.backend = nil
	c.locker = nil
	c.mu.Unlock()

	if registration != nil {
		if err := registration.Close(ctx); err != nil {
			go finishBackendClose(registration, backend, locker)
			return err
		}
	}
	return finishBackendResources(backend, locker)
}

func finishBackendClose(
	registration *redfishruntime.BackendRegistration,
	backend *journalBackend,
	locker *filelock.Locker,
) {
	_ = registration.Close(context.Background())
	_ = finishBackendResources(backend, locker)
}

func finishBackendResources(backend *journalBackend, locker *filelock.Locker) error {
	var result error
	if backend != nil {
		result = errors.Join(result, backend.Close())
	}
	if locker != nil {
		locker.UnlockAll()
	}
	return result
}

func (c *Collector) Collect(context.Context) error {
	labels := []string{c.key, c.jobName}
	state := "unavailable"
	if c.ready.Load() {
		state = "ready"
	}
	c.metrics.state.WithLabelValues(labels...).Enable(state)
	c.metrics.storageTarget.WithLabelValues(labels...).Observe(float64(c.maxBytes))

	c.mu.Lock()
	backend := c.backend
	dir := c.dir
	c.mu.Unlock()
	var stats directoryStats
	statsOK := false
	if dir != "" {
		var err error
		stats, err = scanJournalDirectory(dir)
		if err != nil {
			c.Limit("redfish-logs:directory-scan", 1, time.Hour).
				Warningf("Redfish log backend %q cannot inspect journal storage: %v", c.jobName, err)
		} else {
			statsOK = true
		}
	}
	if statsOK {
		c.metrics.storageUsed.WithLabelValues(labels...).Observe(float64(stats.bytes))
		c.metrics.filesActive.WithLabelValues(labels...).Observe(float64(stats.active))
		c.metrics.filesArchived.WithLabelValues(labels...).Observe(float64(stats.archived))
	}
	c.metrics.producersActive.WithLabelValues(labels...).Observe(float64(c.runtime.BackendProducerCount(c.jobName)))

	if backend != nil {
		c.metrics.pipelineReceived.WithLabelValues(labels...).ObserveTotal(float64(backend.received.Load()))
		c.metrics.pipelineCommitted.WithLabelValues(labels...).ObserveTotal(float64(backend.committed.Load()))
		c.metrics.pipelineDuplicates.WithLabelValues(labels...).ObserveTotal(float64(backend.duplicates.Load()))
		c.metrics.pipelineWriteFailed.WithLabelValues(labels...).ObserveTotal(float64(backend.writeFailed.Load()))
		c.metrics.retentionFiles.WithLabelValues(labels...).ObserveTotal(float64(backend.retentionFiles.Load()))
		c.metrics.retentionBytes.WithLabelValues(labels...).ObserveTotal(float64(backend.retentionBytes.Load()))
		c.metrics.syncDuration.WithLabelValues(labels...).
			Observe(float64(backend.syncDurationNanos.Load()) / float64(time.Second))
		if failures := backend.retentionFailed.Load(); failures > c.lastRetentionFailures {
			c.lastRetentionFailures = failures
			c.Limit("redfish-logs:retention", 1, time.Hour).
				Warningf("Redfish log backend %q could not enforce its retention target", c.jobName)
		}
	}
	return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = c.close(shutdownCtx)
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
