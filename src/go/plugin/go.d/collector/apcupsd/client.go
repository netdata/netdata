// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"time"
)

type apcupsdConn interface {
	connect() error
	disconnect() error
	status() ([]byte, error)
}

func newUpsdConn(conf Config) apcupsdConn {
	return &apcupsdClient{
		address: conf.Address,
		timeout: conf.Timeout.Duration(),
	}
}

type apcupsdClient struct {
	address string
	timeout time.Duration
	conn    net.Conn
}

func (c *apcupsdClient) connect() error {
	if c.conn != nil {
		_ = c.disconnect()
	}

	conn, err := net.DialTimeout("tcp", c.address, c.timeout)
	if err != nil {
		return err
	}

	c.conn = conn

	return nil
}

func (c *apcupsdClient) disconnect() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *apcupsdClient) status() ([]byte, error) {
	if err := c.send("status"); err != nil {
		return nil, err
	}
	return c.receive()
}

func (c *apcupsdClient) send(cmd string) error {
	// https://github.com/therealbstern/apcupsd/blob/224d19d5faa508d04267f6135fe53d50800550de/src/lib/apclibnis.c#L153

	msgLength := make([]byte, 2)

	binary.BigEndian.PutUint16(msgLength, uint16(len(cmd)))

	if err := c.conn.SetWriteDeadline(c.deadline()); err != nil {
		return err
	}

	if _, err := c.conn.Write(append(msgLength, cmd...)); err != nil {
		return err
	}

	return nil
}

func (c *apcupsdClient) receive() ([]byte, error) {
	// https://github.com/therealbstern/apcupsd/blob/224d19d5faa508d04267f6135fe53d50800550de/src/apcnis.c#L54

	var buf bytes.Buffer
	msgLength := make([]byte, 2)

	for {
		if err := c.conn.SetReadDeadline(c.deadline()); err != nil {
			return nil, err
		}

		if _, err := io.ReadFull(c.conn, msgLength); err != nil {
			return nil, err
		}

		length := binary.BigEndian.Uint16(msgLength)
		if length == 0 {
			break
		}

		if err := c.conn.SetReadDeadline(c.deadline()); err != nil {
			return nil, err
		}

		if _, err := io.CopyN(&buf, c.conn, int64(length)); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (c *apcupsdClient) deadline() time.Time {
	return time.Now().Add(c.timeout)
}
