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
	cfg := socket.Config{
		Address: c.address,
		Timeout: c.timeout,
	}

	var s string
	err := socket.ConnectAndRead(cfg, func(bs []byte) (bool, error) {
		s = string(bs)
		return false, nil
	})
	if err != nil {
		return "", err
	}

	return s, nil
}
