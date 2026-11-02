// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	_ "embed"
	"errors"
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

	store      metrix.CollectorStore
	metrics    *collectorMetrics
	httpClient *http.Client
	client     *merakiapi.Client
	notBefore  time.Time
	now        func() time.Time
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(ctx context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	return c.initClient(ctx)
}

func (c *Collector) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	if err := c.ready(ctx); err != nil {
		return err
	}
	if err := c.client.ProbeOrganization(ctx, c.OrganizationID); err != nil {
		return c.operationError(err)
	}
	return ctx.Err()
}

func (c *Collector) Collect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	if err := c.ready(ctx); err != nil {
		return err
	}
	if err := c.collect(ctx); err != nil {
		return c.operationError(err)
	}
	return ctx.Err()
}

func (c *Collector) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.client == nil {
		return errors.New("Meraki client is not initialized")
	}
	if c.now().Before(c.notBefore) {
		return errors.New("Meraki API Retry-After delay has not elapsed")
	}
	return nil
}

func (c *Collector) operationError(err error) error {
	var apiErr *merakiapi.HTTPError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests && apiErr.HasRetryAfter {
		c.notBefore = c.now().Add(apiErr.RetryAfter)
	}
	return err
}

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplateYAML }
