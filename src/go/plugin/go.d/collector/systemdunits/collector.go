// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"

	"github.com/coreos/go-systemd/v22/dbus"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("systemdunits", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10, // gathering systemd units can be a CPU intensive op
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout:               confopt.Duration(time.Second * 2),
			Include:               []string{"*.service"},
			SkipTransient:         false,
			CollectUnitFiles:      false,
			IncludeUnitFiles:      []string{"*.service"},
			CollectUnitFilesEvery: confopt.Duration(time.Minute * 5),
		},
		charts:        &module.Charts{},
		client:        newSystemdDBusClient(),
		seenUnits:     make(map[string]bool),
		unitTransient: make(map[string]bool),
		seenUnitFiles: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery           int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout               confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Include               []string         `yaml:"include,omitempty" json:"include"`
	SkipTransient         bool             `yaml:"skip_transient" json:"skip_transient"`
	CollectUnitFiles      bool             `yaml:"collect_unit_files" json:"collect_unit_files"`
	IncludeUnitFiles      []string         `yaml:"include_unit_files,omitempty" json:"include_unit_files"`
	CollectUnitFilesEvery confopt.Duration `yaml:"collect_unit_files_every,omitempty" json:"collect_unit_files_every"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	client systemdClient
	conn   systemdConnection

	systemdVersion int

	seenUnits     map[string]bool
	unitTransient map[string]bool
	unitSr        matcher.Matcher

	lastListUnitFilesTime time.Time
	cachedUnitFiles       []dbus.UnitFile
	seenUnitFiles         map[string]bool

	charts *module.Charts
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	sr, err := c.initUnitSelector()
	if err != nil {
		return fmt.Errorf("init unit selector: %v", err)
	}
	c.unitSr = sr

	c.Debugf("timeout: %s", c.Timeout)
	c.Debugf("units: patterns '%v'", c.Include)
	c.Debugf("unit files: enabled '%v', every '%s', patterns: %v",
		c.CollectUnitFiles, c.CollectUnitFilesEvery, c.IncludeUnitFiles)

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

func (c *Collector) Cleanup(context.Context) {
	c.closeConnection()
}
