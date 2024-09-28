// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	"bytes"
	"fmt"
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
	var n int64
	var err error
	const readLineLimit = 1000 * 10

	cfg := socket.Config{
		Address: c.address,
		Timeout: c.timeout,
	}

	clientErr := socket.ConnectAndRead(cfg, func(bs []byte) bool {
		b.Write(bs)
		b.WriteByte('\n')

		if n++; n >= readLineLimit {
			err = fmt.Errorf("read line limit exceeded %d", readLineLimit)
			return false
		}
		// The server will close the connection when it has finished sending data.
		return true
	})
	if clientErr != nil {
		return nil, clientErr
	}
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
