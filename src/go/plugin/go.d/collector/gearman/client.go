// SPDX-License-Identifier: GPL-3.0-or-later

package gearman

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

type gearmanConn interface {
	connect() error
	disconnect()
	queryStatus() ([]byte, error)
	queryPriorityStatus() ([]byte, error)
}

func newGearmanConn(conf Config) gearmanConn {
	return &gearmanClient{conn: socket.New(socket.Config{
		Address:      conf.Address,
		Timeout:      conf.Timeout.Duration(),
		MaxReadLines: 10000,
	})}
}

type gearmanClient struct {
	conn socket.Client
}

func (c *gearmanClient) connect() error {
	return c.conn.Connect()
}

func (c *gearmanClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *gearmanClient) queryStatus() ([]byte, error) {
	return c.query("status")
}

func (c *gearmanClient) queryPriorityStatus() ([]byte, error) {
	return c.query("prioritystatus")
}

func (c *gearmanClient) query(cmd string) ([]byte, error) {
	var b bytes.Buffer

	if err := c.conn.Command(cmd+"\n", func(bs []byte) (bool, error) {
		s := string(bs)

		if strings.HasPrefix(s, "ERR") {
			return false, fmt.Errorf("command '%s': %s", cmd, s)
		}

		b.WriteString(s)
		b.WriteByte('\n')

		return !strings.HasPrefix(s, "."), nil
	}); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
