// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("x509check", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		CreateV2:   func() collectorapi.CollectorV2 { return New() },
		Config:     func() any { return &Config{} },
		StoreFirst: true,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		store:            store,
		metrics:          newCollectorMetrics(store),
		now:              time.Now,
		revocationStatus: newRevocationChecker().status,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	provider certificateProvider

	store   metrix.CollectorStore
	metrics collectorMetrics

	now              func() time.Time
	revocationStatus func(
		ctx context.Context,
		cert *x509.Certificate,
		issuers []*x509.Certificate,
		now time.Time,
	) (bool, error)
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	provider, err := c.initCertificateProvider(ctx)
	if err != nil {
		return fmt.Errorf("certificate provider init: %w", err)
	}
	c.provider = provider

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	_, err := c.fetchCertificates(ctx)
	return err
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore {
	return c.store
}

func (c *Collector) ChartTemplateYAML() string {
	return chartTemplateYAML
}
