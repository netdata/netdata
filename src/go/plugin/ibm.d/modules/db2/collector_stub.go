// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package db2

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type Collector struct {
	module.Base
}

func New() *Collector {
	return &Collector{}
}

func (c *Collector) Configuration() any { return nil }

func (c *Collector) Init(context.Context) error {
	return errors.New("db2 collector requires CGO support")
}

func (c *Collector) Check(context.Context) error {
	return errors.New("db2 collector requires CGO support")
}

func (c *Collector) Charts() *module.Charts { return nil }

func (c *Collector) Collect(context.Context) map[string]int64 { return nil }

func (c *Collector) Cleanup(context.Context) {}
