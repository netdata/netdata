// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

func newHddTempConn(conf Config) hddtempConn {
	return &hddtempClient{conn: socket.New(socket.Config{
		Address:        conf.Address,
		ConnectTimeout: conf.Timeout.Duration(),
		ReadTimeout:    conf.Timeout.Duration(),
		WriteTimeout:   conf.Timeout.Duration(),
	})}
}

type hddtempClient struct {
	conn socket.Client
}

func (c *hddtempClient) connect() error {
	return c.conn.Connect()
}

func (c *hddtempClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *hddtempClient) queryHddTemp() (string, error) {
	var i int
	var s string
	err := c.conn.Command("", func(bytes []byte) bool {
		if i++; i > 1 {
			return false
		}
		s = string(bytes)
		return true
	})
	if err != nil {
		return "", err
	}
	return s, nil
}
