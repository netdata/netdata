// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/blang/semver/v4"
	"github.com/go-redis/redis/v8"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pika", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Pika {
	return &Pika{
		Config: Config{
			Address: "redis://@localhost:9221",
			Timeout: web.Duration(time.Second),
		},

		collectedCommands: make(map[string]bool),
		collectedDbs:      make(map[string]bool),
	}
}

type Config struct {
	tlscfg.TLSConfig `yaml:",inline" json:""`
	UpdateEvery      int          `yaml:"update_every" json:"update_every"`
	Address          string       `yaml:"address" json:"address"`
	Timeout          web.Duration `yaml:"timeout" json:"timeout"`
}

type (
	Pika struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		pdb redisClient

		server            string
		version           *semver.Version
		collectedCommands map[string]bool
		collectedDbs      map[string]bool
	}
	redisClient interface {
		Info(ctx context.Context, section ...string) *redis.StringCmd
		Close() error
	}
)

func (p *Pika) Configuration() any {
	return p.Config
}

func (p *Pika) Init() error {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return err
	}

	pdb, err := p.initRedisClient()
	if err != nil {
		p.Errorf("init redis client: %v", err)
		return err
	}
	p.pdb = pdb

	charts, err := p.initCharts()
	if err != nil {
		p.Errorf("init charts: %v", err)
		return err
	}
	p.charts = charts

	return nil
}

func (p *Pika) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (p *Pika) Charts() *module.Charts {
	return p.charts
}

func (p *Pika) Collect() map[string]int64 {
	ms, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (p *Pika) Cleanup() {
	if p.pdb == nil {
		return
	}
	err := p.pdb.Close()
	if err != nil {
		p.Warningf("cleanup: error on closing redis client [%s]: %v", p.Address, err)
	}
	p.pdb = nil
}
