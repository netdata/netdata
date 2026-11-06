// SPDX-License-Identifier: GPL-3.0-or-later

package x509check

import (
	"context"
	"crypto/x509"
	_ "embed"
	"fmt"
	"time"

	cfssllog "github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/revoke"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplate string

func init() {
	cfssllog.Level = cfssllog.LevelFatal
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
			Timeout:        confopt.Duration(time.Second * 2),
			CheckFullChain: false,
		},

		store:             store,
		metrics:           newCollectorMetrics(store),
		now:               time.Now,
		verifyCertificate: revoke.VerifyCertificateError,
	}
}

type Config struct {
	Vnode            string           `yaml:"vnode,omitempty"         json:"vnode"`
	UpdateEvery      int              `yaml:"update_every,omitempty"  json:"update_every"`
	Source           string           `yaml:"source"                  json:"source"`
	Timeout          confopt.Duration `yaml:"timeout,omitempty"       json:"timeout"`
	CheckFullChain   bool             `yaml:"check_full_chain"        json:"check_full_chain"`
	CheckRevocation  bool             `yaml:"check_revocation_status" json:"check_revocation_status"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	prov provider

	store             metrix.CollectorStore
	metrics           collectorMetrics
	now               func() time.Time
	verifyCertificate func(*x509.Certificate) (revoked, ok bool, err error)
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prov, err := c.initProvider(ctx)
	if err != nil {
		return fmt.Errorf("certificate provider init: %v", err)
	}
	c.prov = prov

	return nil
}

func (c *Collector) Check(context.Context) error {
	_, err := c.certificates()
	return err
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplate }

func (c *Collector) Collect(context.Context) error { return c.collect() }

func (c *Collector) Cleanup(context.Context) {}
