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
		Address:        conf.Address,
		ConnectTimeout: conf.Timeout.Duration(),
		ReadTimeout:    conf.Timeout.Duration(),
		WriteTimeout:   conf.Timeout.Duration(),
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
	const limitReadLines = 10000
	var num int
	var err error
	var b bytes.Buffer

	clientErr := c.conn.Command(cmd+"\n", func(bs []byte) bool {
		s := string(bs)

		if strings.HasPrefix(s, "ERR") {
			err = fmt.Errorf("command '%s': %s", cmd, s)
			return false
		}

		b.WriteString(s)
		b.WriteByte('\n')

		if num++; num >= limitReadLines {
			err = fmt.Errorf("command '%s': read line limit exceeded (%d)", cmd, limitReadLines)
			return false
		}
		return !strings.HasPrefix(s, ".")
	})
	if clientErr != nil {
		return nil, fmt.Errorf("command '%s' client error: %v", cmd, clientErr)
	}
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
