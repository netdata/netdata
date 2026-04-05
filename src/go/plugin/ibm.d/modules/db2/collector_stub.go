// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !cgo

package db2

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type Collector struct {
	collectorapi.Base
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

func (c *Collector) Charts() *collectorapi.Charts { return nil }

func (c *Collector) Collect(context.Context) map[string]int64 { return nil }

func (c *Collector) Cleanup(context.Context) {}
