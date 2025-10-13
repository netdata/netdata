// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("smartctl", module.Creator{
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
			Timeout:          confopt.Duration(time.Second * 5),
			ScanEvery:        confopt.Duration(time.Minute * 15),
			PollDevicesEvery: confopt.Duration(time.Minute * 5),
			NoCheckPowerMode: "standby",
			DeviceSelector:   "*",
			ConcurrentScans:  0, // Default to sequential
		},
		charts:      &module.Charts{},
		forceScan:   true,
		deviceSr:    matcher.TRUE(),
		seenDevices: make(map[string]bool),
	}
}

type (
	Config struct {
		UpdateEvery      int                 `yaml:"update_every,omitempty" json:"update_every"`
		Timeout          confopt.Duration    `yaml:"timeout,omitempty" json:"timeout"`
		ScanEvery        confopt.Duration    `yaml:"scan_every,omitempty" json:"scan_every"`
		PollDevicesEvery confopt.Duration    `yaml:"poll_devices_every,omitempty" json:"poll_devices_every"`
		NoCheckPowerMode string              `yaml:"no_check_power_mode,omitempty" json:"no_check_power_mode"`
		DeviceSelector   string              `yaml:"device_selector,omitempty" json:"device_selector"`
		ExtraDevices     []ConfigExtraDevice `yaml:"extra_devices,omitempty" json:"extra_devices"`
		ConcurrentScans  int                 `yaml:"concurrent_scans,omitempty" json:"concurrent_scans"`
	}
	ConfigExtraDevice struct {
		Name string `yaml:"name" json:"name"`
		Type string `yaml:"type" json:"type"`
	}
)

type Collector struct {
	module.Base
	Config `yaml:",inline" data:""`

	charts *module.Charts

	exec smartctlCli

	deviceSr matcher.Matcher

	lastScanTime   time.Time
	forceScan      bool
	scannedDevices map[string]*scanDevice

	lastDevicePollTime time.Time
	forceDevicePoll    bool

	seenDevices map[string]bool
	mx          map[string]int64
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %s", err)
	}

	sr, err := c.initDeviceSelector()
	if err != nil {
		return fmt.Errorf("device selector initialization: %v", err)
	}
	c.deviceSr = sr

	smartctlExec, err := c.initSmartctlCli()
	if err != nil {
		return fmt.Errorf("smartctl exec initialization: %v", err)
	}
	c.exec = smartctlExec

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
