// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

func newMemcachedConn(conf Config) memcachedConn {
	return &memcachedClient{conn: socket.New(socket.Config{
		Address:        conf.Address,
		ConnectTimeout: conf.Timeout.Duration(),
		ReadTimeout:    conf.Timeout.Duration(),
		WriteTimeout:   conf.Timeout.Duration(),
	})}
}

type memcachedClient struct {
	conn socket.Client
}

func (c *memcachedClient) connect() error {
	return c.conn.Connect()
}

func (c *memcachedClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *memcachedClient) queryStats() (string, error) {
    var s string
    err := c.conn.Command("stats\r\n", func(bytes []byte) bool {
        s += string(bytes)
        // Stop processing after we receive the full response
        return !strings.Contains(s, "END")
    })
    if err != nil {
        return "", err
    }
    return s, nil
}