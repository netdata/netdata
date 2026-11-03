// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	_ "embed"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

func init() {
	collectorapi.Register("meraki", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			URL:         merakiapi.DefaultBaseURL,
			UpdateEvery: defaultUpdateEvery,
			Timeout:     defaultTimeout,
		},
		store:   store,
		metrics: newCollectorMetrics(store),
		now:     time.Now,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	httpClient *http.Client
	client     *merakiapi.Client

	// retryNotBefore is the end of the last Retry-After wait Meraki requested.
	retryNotBefore time.Time
	now            func() time.Time
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	return c.initClient(ctx)
}

func (c *Collector) Check(ctx context.Context) error {
	if err := c.checkRetryAfter(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	return c.rememberRetryAfter(c.client.ProbeOrganization(ctx, c.OrganizationID))
}

func (c *Collector) Collect(ctx context.Context) error {
	if err := c.checkRetryAfter(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	return c.rememberRetryAfter(c.collect(ctx))
}

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
