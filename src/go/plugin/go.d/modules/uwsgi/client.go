// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	"bytes"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type uwsgiConn interface {
	connect() error
	disconnect()
	queryStats() ([]byte, error)
}

func newUwsgiConn(conf Config) uwsgiConn {
	return &uwsgiClient{conn: socket.New(socket.Config{
		Address:        conf.Address,
		ConnectTimeout: conf.Timeout.Duration(),
		ReadTimeout:    conf.Timeout.Duration(),
		WriteTimeout:   conf.Timeout.Duration(),
	})}
}

type uwsgiClient struct {
	conn socket.Client
}

func (c *uwsgiClient) connect() error {
	return c.conn.Connect()
}

func (c *uwsgiClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *uwsgiClient) queryStats() ([]byte, error) {
	var b bytes.Buffer
	var n int64
	var err error
	const readLineLimit = 1000 * 10

	clientErr := c.conn.Command("", func(bs []byte) bool {
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
