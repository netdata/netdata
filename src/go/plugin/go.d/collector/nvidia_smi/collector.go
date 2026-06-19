// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"context"
	_ "embed"
	"errors"
	"runtime"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("nvidia_smi", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 10),
			// Disable loop mode on Windows due to go.d.plugin's non-graceful exit
			// which can leave `nvidia_smi` processes running indefinitely.
			LoopMode: !(runtime.GOOS == "windows"),
		},
		binName: "nvidia-smi",
		charts:  &collectorapi.Charts{},
		gpus:    make(map[string]bool),
		migs:    make(map[string]bool),
	}

}

type Config struct {
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath         string           `yaml:"binary_path" json:"binary_path"`
	LoopMode           bool             `yaml:"loop_mode,omitempty" json:"loop_mode"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	exec    nvidiaSmiBinary
	binName string

	gpus map[string]bool
	migs map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.exec == nil {
		if runtime.GOOS == "windows" && c.LoopMode {
			c.LoopMode = false
		}
		smi, err := c.initNvidiaSmiExec()
		if err != nil {
			return err
		}
		c.exec = smi
	}

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

func (c *Collector) Charts() *collectorapi.Charts {
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
	if c.exec != nil {
		if err := c.exec.stop(); err != nil {
			c.Errorf("cleanup: %v", err)
		}
		c.exec = nil
	}
}
