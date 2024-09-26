// SPDX-License-Identifier: GPL-3.0-or-later

package hddtemp

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type hddtempConn interface {
	queryHddTemp() (string, error)
}

func newHddTempConn(conf Config) hddtempConn {
	return &hddtempClient{
		address: conf.Address,
		timeout: conf.Timeout.Duration(),
	}
}

type hddtempClient struct {
	address string
	timeout time.Duration
}

func (c *hddtempClient) queryHddTemp() (string, error) {
	var i int
	var s string

	cfg := socket.Config{
		Address:        c.address,
		ConnectTimeout: c.timeout,
		ReadTimeout:    c.timeout,
		WriteTimeout:   c.timeout,
	}

	err := socket.ConnectAndRead(cfg, func(bs []byte) bool {
		if i++; i > 1 {
			return false
		}
		s = string(bs)
		return true

	})
	if err != nil {
		return "", err
	}

	return s, nil
}
