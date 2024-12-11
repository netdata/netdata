// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	"bytes"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type uwsgiConn interface {
	queryStats() ([]byte, error)
}

func newUwsgiConn(conf Config) uwsgiConn {
	return &uwsgiClient{
		address: conf.Address,
		timeout: conf.Timeout.Duration(),
	}
}

type uwsgiClient struct {
	address string
	timeout time.Duration
}

func (c *uwsgiClient) queryStats() ([]byte, error) {
	var b bytes.Buffer

	cfg := socket.Config{
		Address:      c.address,
		Timeout:      c.timeout,
		MaxReadLines: 1000 * 10,
	}

	if err := socket.ConnectAndRead(cfg, func(bs []byte) (bool, error) {
		b.Write(bs)
		b.WriteByte('\n')
		// The server will close the connection when it has finished sending data.
		return true, nil
	}); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
