// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"errors"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/tlscfg"

	"github.com/go-redis/redis/v8"
)

func (r *Redis) validateConfig() error {
	if r.Address == "" {
		return errors.New("'address' not set")
	}
	return nil
}

func (r *Redis) initRedisClient() (*redis.Client, error) {
	opts, err := redis.ParseURL(r.Address)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := tlscfg.NewTLSConfig(r.TLSConfig)
	if err != nil {
		return nil, err
	}

	if opts.TLSConfig != nil && tlsConfig != nil {
		tlsConfig.ServerName = opts.TLSConfig.ServerName
	}

	if opts.Username == "" && r.Username != "" {
		opts.Username = r.Username
	}
	if opts.Password == "" && r.Password != "" {
		opts.Password = r.Password
	}

	opts.PoolSize = 1
	opts.TLSConfig = tlsConfig
	opts.DialTimeout = r.Timeout.Duration
	opts.ReadTimeout = r.Timeout.Duration
	opts.WriteTimeout = r.Timeout.Duration

	return redis.NewClient(opts), nil
}

func (r *Redis) initCharts() (*module.Charts, error) {
	return redisCharts.Copy(), nil
}
