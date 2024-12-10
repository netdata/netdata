// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("hpssa", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:          &module.Charts{},
		seenControllers: make(map[string]*hpssaController),
		seenArrays:      make(map[string]*hpssaArray),
		seenLDrives:     make(map[string]*hpssaLogicalDrive),
		seenPDrives:     make(map[string]*hpssaPhysicalDrive),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec ssacliBinary

	seenControllers map[string]*hpssaController
	seenArrays      map[string]*hpssaArray
	seenLDrives     map[string]*hpssaLogicalDrive
	seenPDrives     map[string]*hpssaPhysicalDrive
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	ssacli, err := c.initSsacliBinary()
	if err != nil {
		return fmt.Errorf("ssacli exec initialization: %v", err)
	}
	c.exec = ssacli

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {}
