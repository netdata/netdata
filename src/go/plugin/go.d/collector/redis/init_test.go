// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
)

func TestCollector_initRedisClient(t *testing.T) {
	type clientOptions struct {
		username           string
		password           string
		tls                bool
		serverName         string
		insecureSkipVerify bool
		poolSize           int
		timeouts           [3]time.Duration // dial, read, write
	}
	defaults := func(o clientOptions) clientOptions {
		o.poolSize = 1
		o.timeouts = [3]time.Duration{2 * time.Second, 2 * time.Second, 2 * time.Second}
		return o
	}

	tests := map[string]struct {
		config Config
		want   clientOptions
	}{
		"credentials in the address": {
			config: Config{
				Address: "redis://addr-user:addr-pass@localhost:6379",
			},
			want: defaults(clientOptions{
				username: "addr-user",
				password: "addr-pass",
			}),
		},
		"credentials in the options": {
			config: Config{
				Address:  "redis://localhost:6379",
				Username: "opt-user",
				Password: "opt-pass",
			},
			want: defaults(clientOptions{
				username: "opt-user",
				password: "opt-pass",
			}),
		},
		"address credentials take precedence": {
			config: Config{
				Address:  "redis://addr-user:addr-pass@localhost:6379",
				Username: "opt-user",
				Password: "opt-pass",
			},
			want: defaults(clientOptions{
				username: "addr-user",
				password: "addr-pass",
			}),
		},
		"rediss address enables TLS": {
			config: Config{
				Address: "rediss://redis.example:6379",
			},
			want: defaults(clientOptions{
				tls:        true,
				serverName: "redis.example",
			}),
		},
		"rediss address keeps its server name with TLS options": {
			config: Config{
				Address: "rediss://redis.example:6379",
				TLSConfig: tlscfg.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
			want: defaults(clientOptions{
				tls:                true,
				serverName:         "redis.example",
				insecureSkipVerify: true,
			}),
		},
		"any TLS option enables TLS for a redis address": {
			config: Config{
				Address: "redis://redis.example:6379",
				TLSConfig: tlscfg.TLSConfig{
					InsecureSkipVerify: true,
				},
			},
			want: defaults(clientOptions{
				tls:                true,
				insecureSkipVerify: true,
			}),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config
			collr.Timeout = confopt.Duration(2 * time.Second)

			rdb, err := collr.initRedisClient(context.Background())
			require.NoError(t, err)
			defer func() { _ = rdb.Close() }()

			opts := rdb.Options()
			got := clientOptions{
				username: opts.Username,
				password: opts.Password,
				tls:      opts.TLSConfig != nil,
				poolSize: opts.PoolSize,
				timeouts: [3]time.Duration{opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout},
			}
			if opts.TLSConfig != nil {
				got.serverName = opts.TLSConfig.ServerName
				got.insecureSkipVerify = opts.TLSConfig.InsecureSkipVerify
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCollector_InitErrorHidesAddressCredentials(t *testing.T) {
	for name, address := range map[string]string{
		"invalid port":   "redis://user:s3cret@localhost:63 79",
		"invalid escape": "redis://user:s3cret@local%zzhost:6379",
	} {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Address = address

			err := collr.Init(context.Background())

			require.Error(t, err)
			assert.NotContains(t, err.Error(), "s3cret")
		})
	}
}
