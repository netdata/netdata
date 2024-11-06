// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("zookeeper", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Zookeeper {
	return &Zookeeper{
		Config: Config{
			Address: "127.0.0.1:2181",
			Timeout: confopt.Duration(time.Second),
			UseTLS:  false,
		}}
}

type Config struct {
	UpdateEvery      int              `yaml:"update_every,omitempty" json:"update_every"`
	Address          string           `yaml:"address" json:"address"`
	Timeout          confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
	UseTLS           bool `yaml:"use_tls,omitempty" json:"use_tls"`
}

type (
	Zookeeper struct {
		module.Base
		Config `yaml:",inline" json:""`

		fetcher
	}
	fetcher interface {
		fetch(command string) ([]string, error)
	}
)

func (z *Zookeeper) Configuration() any {
	return z.Config
}

func (z *Zookeeper) Init() error {
	if err := z.verifyConfig(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	f, err := z.initZookeeperFetcher()
	if err != nil {
		return fmt.Errorf("init zookeeper fetcher: %v", err)
	}
	z.fetcher = f

	return nil
}

func (z *Zookeeper) Check() error {
	mx, err := z.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (z *Zookeeper) Charts() *Charts {
	return charts.Copy()
}

func (z *Zookeeper) Collect() map[string]int64 {
	mx, err := z.collect()
	if err != nil {
		z.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (z *Zookeeper) Cleanup() {}
