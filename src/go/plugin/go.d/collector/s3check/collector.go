// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("s3check", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        defaultUpdateEvery,
			AutoDetectionRetry: 0,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			UpdateEvery:        defaultUpdateEvery,
			Prefix:             defaultPrefix,
			PathStyle:          true,
			MaxRetries:         defaultMaxRetries,
			LatencyThresholdMS: 0,
			ClientConfig: web.ClientConfig{
				Timeout:           confopt.Duration(defaultTimeout),
				NotFollowRedirect: true,
			},
		},
		store:      metrix.NewCollectorStore(),
		stats:      newStageStats(),
		now:        time.Now,
		randomRead: readRandomBytes,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore

	httpClient *http.Client
	client     s3Client
	newClient  func(Config) (*http.Client, s3Client, error)

	stats stageStats

	now        func() time.Time
	randomRead func(int) ([]byte, error)

	currentKey       string
	objectMayExist   bool
	cleanupCompleted bool
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	if c.newClient == nil {
		c.newClient = newAWSS3Client
	}

	httpClient, client, err := c.newClient(c.Config)
	if err != nil {
		return fmt.Errorf("create S3 client: %w", err)
	}
	c.httpClient = httpClient
	c.client = client
	c.stats = newStageStats()

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.client == nil {
		return errors.New("S3 client is not initialized")
	}

	status, _, err := c.client.GetBucketVersioning(ctx, c.Bucket)
	if err != nil {
		return errors.New("S3 bucket versioning check failed; verify the endpoint, credentials, bucket, and s3:GetBucketVersioning permission")
	}
	if status != "" {
		return fmt.Errorf("unsupported S3 bucket versioning status %q; use a dedicated unversioned bucket", status)
	}
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	if c.client == nil {
		return errors.New("S3 client is not initialized")
	}

	cycleCtx, cancel := context.WithTimeout(ctx, c.worstCaseDuration())
	defer cancel()

	results := newStageResults()
	c.collect(cycleCtx, results)
	c.writeMetrics(results)

	// A collector-local deadline is represented as a stage timeout. Only cancellation of
	// the runtime-provided context aborts the metric cycle.
	return ctx.Err()
}

func (c *Collector) Cleanup(_ context.Context) {
	if c.client != nil && c.currentKey != "" && c.objectMayExist && !c.cleanupCompleted {
		// Runtime cancellation may already have propagated; cleanup still gets its own
		// bounded deadline so a reachable write path is not abandoned merely by shutdown.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), c.shutdownCleanupDuration())
		_ = c.ensureObjectGone(cleanupCtx, c.currentKey)
		cancel()
	}
	c.currentKey = ""
	c.objectMayExist = false
	c.cleanupCompleted = true

	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
