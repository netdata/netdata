// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"bytes"
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

func (c *memcachedClient) queryStats() ([]byte, error) {
	var b bytes.Buffer
	err := c.conn.Command("stats\r\n", func(bytes []byte) bool {
		s := strings.TrimSpace(string(bytes))
		b.WriteString(s)
		b.WriteByte('\n')
		return !(strings.HasPrefix(s, "END") || strings.HasPrefix(s, "ERROR"))
	})
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
