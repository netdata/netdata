// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/metrics"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/blang/semver/v4"
	"github.com/go-redis/redis/v8"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("redis", module.Creator{
		Create:          func() module.Module { return New() },
		JobConfigSchema: configSchema,
	})
}

func New() *Redis {
	return &Redis{
		Config: Config{
			Address:     "redis://@localhost:6379",
			Timeout:     web.Duration(time.Second),
			PingSamples: 5,
		},

		addAOFChartsOnce:       &sync.Once{},
		addReplSlaveChartsOnce: &sync.Once{},
		pingSummary:            metrics.NewSummary(),
		collectedCommands:      make(map[string]bool),
		collectedDbs:           make(map[string]bool),
	}
}

type Config struct {
	tlscfg.TLSConfig `yaml:",inline" json:""`
	UpdateEvery      int          `yaml:"update_every" json:"update_every"`
	Address          string       `yaml:"address" json:"address"`
	Timeout          web.Duration `yaml:"timeout" json:"timeout"`
	Username         string       `yaml:"username" json:"username"`
	Password         string       `yaml:"password" json:"password"`
	PingSamples      int          `yaml:"ping_samples" json:"ping_samples"`
}

type (
	Redis struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts                 *module.Charts
		addAOFChartsOnce       *sync.Once
		addReplSlaveChartsOnce *sync.Once

		rdb redisClient

		server            string
		version           *semver.Version
		pingSummary       metrics.Summary
		collectedCommands map[string]bool
		collectedDbs      map[string]bool
	}
	redisClient interface {
		Info(ctx context.Context, section ...string) *redis.StringCmd
		Ping(context.Context) *redis.StatusCmd
		Close() error
	}
)

func (r *Redis) Configuration() any {
	return r.Config
}

func (r *Redis) Init() error {
	err := r.validateConfig()
	if err != nil {
		r.Errorf("config validation: %v", err)
		return err
	}

	rdb, err := r.initRedisClient()
	if err != nil {
		r.Errorf("init redis client: %v", err)
		return err
	}
	r.rdb = rdb

	charts, err := r.initCharts()
	if err != nil {
		r.Errorf("init charts: %v", err)
		return err
	}
	r.charts = charts

	return nil
}

func (r *Redis) Check() error {
	mx, err := r.collect()
	if err != nil {
		r.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (r *Redis) Charts() *module.Charts {
	return r.charts
}

func (r *Redis) Collect() map[string]int64 {
	ms, err := r.collect()
	if err != nil {
		r.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (r *Redis) Cleanup() {
	if r.rdb == nil {
		return
	}
	err := r.rdb.Close()
	if err != nil {
		r.Warningf("cleanup: error on closing redis client [%s]: %v", r.Address, err)
	}
	r.rdb = nil
}
