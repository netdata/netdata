// SPDX-License-Identifier: GPL-3.0-or-later

package spigotmc

import (
	"time"

	"github.com/gorcon/rcon"
)

type rconConn interface {
	connect() error
	disconnect() error
	queryTps() (string, error)
	queryList() (string, error)
}

const (
	cmdTPS  = "tps"
	cmdList = "list"
)

func newRconConn(cfg Config) rconConn {
	return &rconClient{
		addr:     cfg.Address,
		password: cfg.Password,
		timeout:  cfg.Timeout.Duration(),
	}
}

type rconClient struct {
	conn     *rcon.Conn
	addr     string
	password string
	timeout  time.Duration
}

func (c *rconClient) queryTps() (string, error) {
	return c.query(cmdTPS)
}

func (c *rconClient) queryList() (string, error) {
	return c.query(cmdList)
}

func (c *rconClient) query(cmd string) (string, error) {
	resp, err := c.conn.Execute(cmd)
	if err != nil {
		return "", err
	}
	return resp, nil
}

func (c *rconClient) connect() error {
	_ = c.disconnect()

	conn, err := rcon.Dial(c.addr, c.password, rcon.SetDialTimeout(c.timeout), rcon.SetDeadline(c.timeout))
	if err != nil {
		return err
	}

	c.conn = conn

	return nil
}

func (c *rconClient) disconnect() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	return nil
}
