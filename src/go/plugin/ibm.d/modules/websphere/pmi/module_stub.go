//go:build !cgo

package pmi

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var configSchema string

type Collector struct{ collectorapi.Base }

func New() *Collector { return &Collector{} }

func (c *Collector) Configuration() any { return nil }

func (c *Collector) Init(context.Context) error {
	return errors.New("websphere_pmi collector requires CGO support")
}

func (c *Collector) Check(context.Context) error {
	return errors.New("websphere_pmi collector requires CGO support")
}

func (c *Collector) Charts() *collectorapi.Charts { return nil }

func (c *Collector) Collect(context.Context) map[string]int64 { return nil }

func (c *Collector) Cleanup(context.Context) {}

func init() {
	collectorapi.Register("websphere_pmi", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return nil },
	})
}
