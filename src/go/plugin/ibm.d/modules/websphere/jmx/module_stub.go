//go:build !cgo

package jmx

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var configSchema string

// Collector stub for non-CGO builds.
type Collector struct{ module.Base }

func New() *Collector { return &Collector{} }

func (c *Collector) Configuration() any { return nil }

func (c *Collector) Init(context.Context) error {
	return errors.New("websphere_jmx collector requires CGO support")
}

func (c *Collector) Check(context.Context) error {
	return errors.New("websphere_jmx collector requires CGO support")
}

func (c *Collector) Charts() *module.Charts { return nil }

func (c *Collector) Collect(context.Context) map[string]int64 { return nil }

func (c *Collector) Cleanup(context.Context) {}

func init() {
	module.Register("websphere_jmx", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return nil },
	})
}
