// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

const cleanupBudget = 5 * time.Second

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("s3check", collectorapi.Creator{
		JobConfigSchema:    configSchema,
		UpdateEvery:        defaultUpdateEvery,
		AutoDetectionRetry: 0,
		CreateV2:           func() collectorapi.CollectorV2 { return New() },
		Config:             func() any { return &Config{} },
	})
}

func New() *Collector {
	config := Config{
		UpdateEvery: defaultUpdateEvery,
		Mode:        string(contract.ModeLifecycle),
	}
	c := &Collector{
		Config:           config,
		store:            metrix.NewCollectorStore(),
		newS3Client:      s3client.New,
		registryUniqueID: pluginconfig.RegistryUniqueID,
		journalRoot: resolveJournalRoot(
			pluginconfig.VarLibDir(), buildinfo.VarLibDir, buildinfo.DefaultVarLibDir,
		),
	}
	c.metrics = newMetricRenderer(c.store)
	return c
}

type s3ClientFactory func(context.Context, s3client.Config) (s3client.Client, error)

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *metricRenderer
	engine  contract.Engine

	newS3Client      s3ClientFactory
	registryUniqueID func() string
	journalRoot      string
}

func (c *Collector) Configuration() any { return c.Config.withDefaults() }

func (c *Collector) Init(ctx context.Context) error {
	c.Config.applyDefaults()
	selected, err := c.Config.validatedModeConfig()
	if err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	if c.newS3Client == nil {
		return errors.New("S3 client factory is not initialized")
	}
	if c.registryUniqueID == nil {
		return errors.New("Agent identity provider is not initialized")
	}
	agentID := strings.TrimSpace(c.registryUniqueID())
	if agentID == "" {
		return errors.New("persisted Agent registry identity is unavailable")
	}
	engine, err := c.buildEngine(ctx, agentID, selected)
	if err != nil {
		return err
	}
	c.engine = engine
	destination := ""
	if selected.Destination != nil {
		destination = selected.Destination.Name
	}
	c.metrics.reset(selected.Source.Name, destination)
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if c.engine == nil {
		return errors.New("s3check engine is not initialized")
	}
	return c.engine.Check(ctx)
}

func (c *Collector) Collect(ctx context.Context) error {
	if c.engine == nil {
		return errors.New("s3check engine is not initialized")
	}
	result := c.engine.Collect(ctx)
	c.metrics.write(result)
	for _, operation := range result.Operations {
		if operation.Status != contract.StatusFailed || operation.Err == nil {
			continue
		}
		key := fmt.Sprintf("s3check:operation:%s:%s", operation.Endpoint, operation.Name)
		c.Limit(key, 1, time.Minute).Warningf(
			"s3check %s %s failed: %v", operation.Endpoint, operation.Name, operation.Err,
		)
	}
	if result.Err != nil {
		c.Limit("s3check:collection", 1, time.Minute).Warningf("s3check collection failed: %v", result.Err)
	}
	return ctx.Err()
}

func (c *Collector) Cleanup(context.Context) {
	if c.engine == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupBudget)
	defer cancel()
	c.engine.Cleanup(ctx)
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }

func resolveJournalRoot(runtimeVarLibDir, compiledVarLibDir, defaultVarLibDir string) string {
	varLibDir := strings.TrimSpace(runtimeVarLibDir)
	if varLibDir == "" {
		varLibDir = strings.TrimSpace(compiledVarLibDir)
	}
	if varLibDir == "" {
		varLibDir = strings.TrimSpace(defaultVarLibDir)
	}
	return filepath.Join(filepath.Clean(varLibDir), "s3check")
}
