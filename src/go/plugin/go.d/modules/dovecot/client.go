// SPDX-License-Identifier: GPL-3.0-or-later

package dovecot

import (
	"bytes"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type dovecotConn interface {
	connect() error
	disconnect()
	queryExportGlobal() ([]byte, error)
}

func newDovecotConn(conf Config) dovecotConn {
	return &dovecotClient{conn: socket.New(socket.Config{
		Address:        conf.Address,
		ConnectTimeout: conf.Timeout.Duration(),
		ReadTimeout:    conf.Timeout.Duration(),
		WriteTimeout:   conf.Timeout.Duration(),
	})}
}

type dovecotClient struct {
	conn socket.Client
}

func (c *dovecotClient) connect() error {
	return c.conn.Connect()
}

func (c *dovecotClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *dovecotClient) queryExportGlobal() ([]byte, error) {
	var b bytes.Buffer
	var n int

	err := c.conn.Command("EXPORT\tglobal\n", func(bs []byte) bool {
		b.Write(bs)
		b.WriteByte('\n')

		n++
		return n < 2
	})
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
