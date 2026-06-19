// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package as400

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var configSchema string

type Collector struct {
	collectorapi.Base
	Config
}

func New() *Collector {
	return &Collector{}
}

func (c *Collector) Configuration() any {
	return &c.Config
}

func (c *Collector) Init(context.Context) error {
	return errors.New("as400 collector requires CGO support")
}

func (c *Collector) Check(context.Context) error {
	return errors.New("as400 collector requires CGO support")
}

func (c *Collector) Charts() *collectorapi.Charts { return nil }

func (c *Collector) Collect(context.Context) map[string]int64 { return nil }

func (c *Collector) Cleanup(context.Context) {}

func init() {
	collectorapi.Register("as400", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config:          func() any { return nil },
	})
}
