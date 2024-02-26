// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"context"
	_ "embed"
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
			Timeout: web.Duration{Duration: time.Second},
		},

		collectedCommands: make(map[string]bool),
		collectedDbs:      make(map[string]bool),
	}
}

type Config struct {
	Address          string       `yaml:"address"`
	Timeout          web.Duration `yaml:"timeout"`
	tlscfg.TLSConfig `yaml:",inline"`
}

type (
	Pika struct {
		module.Base
		Config `yaml:",inline"`

		pdb redisClient

		server  string
		version *semver.Version

		collectedCommands map[string]bool
		collectedDbs      map[string]bool

		charts *module.Charts
	}
	redisClient interface {
		Info(ctx context.Context, section ...string) *redis.StringCmd
		Close() error
	}
)

func (p *Pika) Init() bool {
	err := p.validateConfig()
	if err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}

	pdb, err := p.initRedisClient()
	if err != nil {
		p.Errorf("init redis client: %v", err)
		return false
	}
	p.pdb = pdb

	charts, err := p.initCharts()
	if err != nil {
		p.Errorf("init charts: %v", err)
		return false
	}
	p.charts = charts

	return true
}

func (p *Pika) Check() bool {
	return len(p.Collect()) > 0
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
