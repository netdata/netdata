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
		Address: conf.Address,
		Timeout: conf.Timeout.Duration(),
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

	if err := c.conn.Command("EXPORT\tglobal\n", func(bs []byte) (bool, error) {
		b.Write(bs)
		b.WriteByte('\n')

		n++
		return n < 2, nil
	}); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
