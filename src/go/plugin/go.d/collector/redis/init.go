// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/redis/go-redis/v9"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		return errors.New("'address' not set")
	}
	return nil
}

func (c *Collector) initRedisClient(ctx context.Context) (*redis.Client, error) {
	opts, err := redis.ParseURL(c.Address)
	if err != nil {
		// url.Error repeats the raw address, which may embed credentials.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("invalid 'address': %w", urlErr.Err)
		}
		return nil, err
	}

	tlsConfig, err := tlscfg.NewTLSConfig(ctx, c.TLSConfig)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		// A rediss:// address sets the server name used for hostname verification.
		if opts.TLSConfig != nil {
			tlsConfig.ServerName = opts.TLSConfig.ServerName
		}
		opts.TLSConfig = tlsConfig
	}

	// Credentials in the address take precedence over the username and password options.
	if opts.Username == "" {
		opts.Username = c.Username
	}
	if opts.Password == "" {
		opts.Password = c.Password
	}

	opts.PoolSize = 1
	opts.DialTimeout = c.Timeout.Duration()
	opts.ReadTimeout = c.Timeout.Duration()
	opts.WriteTimeout = c.Timeout.Duration()

	return redis.NewClient(opts), nil
}
