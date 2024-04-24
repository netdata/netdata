// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/tidwall/gjson"
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
	})
}

func New() *Smartctl {
	return &Smartctl{
		Config: Config{
			Timeout:          web.Duration(time.Second * 5),
			ScanEvery:        web.Duration(time.Minute * 15),
			PollDevicesEvery: web.Duration(time.Minute * 5),
			NoCheckPowerMode: "standby",
		},
		charts:      &module.Charts{},
		seenDevices: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery      int          `yaml:"update_every" json:"update_every"`
	Timeout          web.Duration `yaml:"timeout" json:"timeout"`
	ScanEvery        web.Duration `yaml:"scan_every" json:"scan_every"`
	PollDevicesEvery web.Duration `yaml:"poll_devices_every" json:"poll_devices_every"`
	NoCheckPowerMode string       `yaml:"no_check_power_mode" json:"no_check_power_mode"`
}

type (
	Smartctl struct {
		module.Base
		Config `yaml:",inline" data:""`

		charts *module.Charts

		exec smartctlCli

		lastScanTime   time.Time
		forceScan      bool
		scannedDevices map[string]*scanDevice

		lastDevicePollTime time.Time
		forceDevicePoll    bool

		seenDevices map[string]bool
		mx          map[string]int64
	}
	smartctlCli interface {
		scan() (*gjson.Result, error)
		deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error)
	}
)

func (s *Smartctl) Configuration() any {
	return s.Config
}

func (s *Smartctl) Init() error {
	if err := s.validateConfig(); err != nil {
		s.Errorf("config validation error: %s", err)
		return err
	}

	smartctlExec, err := s.initSmartctlCli()
	if err != nil {
		s.Errorf("smartctl exec initialization: %v", err)
		return err
	}
	s.exec = smartctlExec

	return nil
}

func (s *Smartctl) Check() error {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *Smartctl) Charts() *module.Charts {
	return s.charts
}

func (s *Smartctl) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *Smartctl) Cleanup() {}
