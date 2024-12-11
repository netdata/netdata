// SPDX-License-Identifier: GPL-3.0-or-later

package memcached

import (
	"bytes"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type memcachedConn interface {
	connect() error
	disconnect()
	queryStats() ([]byte, error)
}

func newMemcachedConn(conf Config) memcachedConn {
	return &memcachedClient{conn: socket.New(socket.Config{
		Address: conf.Address,
		Timeout: conf.Timeout.Duration(),
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
	if err := c.conn.Command("stats\r\n", func(bytes []byte) (bool, error) {
		s := strings.TrimSpace(string(bytes))
		b.WriteString(s)
		b.WriteByte('\n')

		return !(strings.HasPrefix(s, "END") || strings.HasPrefix(s, "ERROR")), nil
	}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
