// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("wireguard", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *WireGuard {
	return &WireGuard{
		newWGClient:  func() (wgClient, error) { return wgctrl.New() },
		charts:       &module.Charts{},
		devices:      make(map[string]bool),
		peers:        make(map[string]bool),
		cleanupEvery: time.Minute,
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

type (
	WireGuard struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client      wgClient
		newWGClient func() (wgClient, error)

		cleanupLastTime time.Time
		cleanupEvery    time.Duration
		devices         map[string]bool
		peers           map[string]bool
	}
	wgClient interface {
		Devices() ([]*wgtypes.Device, error)
		Close() error
	}
)

func (w *WireGuard) Configuration() any {
	return w.Config
}

func (w *WireGuard) Init() error {
	return nil
}

func (w *WireGuard) Check() error {
	mx, err := w.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (w *WireGuard) Charts() *module.Charts {
	return w.charts
}

func (w *WireGuard) Collect() map[string]int64 {
	mx, err := w.collect()
	if err != nil {
		w.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (w *WireGuard) Cleanup() {
	if w.client == nil {
		return
	}
	if err := w.client.Close(); err != nil {
		w.Warningf("cleanup: error on closing connection: %v", err)
	}
	w.client = nil
}
