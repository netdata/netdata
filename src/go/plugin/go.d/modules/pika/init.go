// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"

	"github.com/go-redis/redis/v8"
)

func (p *Pika) validateConfig() error {
	if p.Address == "" {
		return errors.New("'address' not set")
	}
	return nil
}

func (p *Pika) initRedisClient() (*redis.Client, error) {
	opts, err := redis.ParseURL(p.Address)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := tlscfg.NewTLSConfig(p.TLSConfig)
	if err != nil {
		return nil, err
	}

	if opts.TLSConfig != nil && tlsConfig != nil {
		tlsConfig.ServerName = opts.TLSConfig.ServerName
	}

	opts.PoolSize = 1
	opts.TLSConfig = tlsConfig
	opts.DialTimeout = p.Timeout.Duration()
	opts.ReadTimeout = p.Timeout.Duration()
	opts.WriteTimeout = p.Timeout.Duration()

	return redis.NewClient(opts), nil
}

func (p *Pika) initCharts() (*module.Charts, error) {
	return pikaCharts.Copy(), nil
}
