// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("zookeeper", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Zookeeper {
	return &Zookeeper{
		Config: Config{
			Address: "127.0.0.1:2181",
			Timeout: web.Duration(time.Second),
			UseTLS:  false,
		}}
}

type Config struct {
	tlscfg.TLSConfig `yaml:",inline" json:""`
	UpdateEvery      int          `yaml:"update_every" json:"update_every"`
	Address          string       `yaml:"address" json:"address"`
	Timeout          web.Duration `yaml:"timeout" json:"timeout"`
	UseTLS           bool         `yaml:"use_tls" json:"use_tls"`
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
		z.Error(err)
		return err
	}

	f, err := z.initZookeeperFetcher()
	if err != nil {
		z.Error(err)
		return err
	}
	z.fetcher = f

	return nil
}

func (z *Zookeeper) Check() error {
	mx, err := z.collect()
	if err != nil {
		z.Error(err)
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
