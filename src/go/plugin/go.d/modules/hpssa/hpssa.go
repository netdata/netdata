// SPDX-License-Identifier: GPL-3.0-or-later

package hpssa

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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

func New() *Hpssa {
	return &Hpssa{
		Config: Config{
			Timeout: web.Duration(time.Second * 2),
		},
		charts:          &module.Charts{},
		seenControllers: make(map[string]*hpssaController),
		seenArrays:      make(map[string]*hpssaArray),
		seenLDrives:     make(map[string]*hpssaLogicalDrive),
		seenPDrives:     make(map[string]*hpssaPhysicalDrive),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	Hpssa struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec ssacli

		seenControllers map[string]*hpssaController
		seenArrays      map[string]*hpssaArray
		seenLDrives     map[string]*hpssaLogicalDrive
		seenPDrives     map[string]*hpssaPhysicalDrive
	}
	ssacli interface {
		controllersInfo() ([]byte, error)
	}
)

func (h *Hpssa) Configuration() any {
	return h.Config
}

func (h *Hpssa) Init() error {
	ssacliExec, err := h.initSsacliExec()
	if err != nil {
		h.Errorf("ssacli exec initialization: %v", err)
		return err
	}
	h.exec = ssacliExec

	return nil
}

func (h *Hpssa) Check() error {
	mx, err := h.collect()
	if err != nil {
		h.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (h *Hpssa) Charts() *module.Charts {
	return h.charts
}

func (h *Hpssa) Collect() map[string]int64 {
	mx, err := h.collect()
	if err != nil {
		h.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (h *Hpssa) Cleanup() {}
