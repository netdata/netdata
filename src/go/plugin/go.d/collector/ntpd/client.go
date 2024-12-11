// SPDX-License-Identifier: GPL-3.0-or-later

package ntpd

import (
	"net"
	"time"

	"github.com/facebook/time/ntp/control"
)

type ntpConn interface {
	systemInfo() (map[string]string, error)
	peerInfo(id uint16) (map[string]string, error)
	peerIDs() ([]uint16, error)
	close()
}

func newNTPClient(c Config) (ntpConn, error) {
	conn, err := net.DialTimeout("udp", c.Address, c.Timeout.Duration())
	if err != nil {
		return nil, err
	}

	client := &ntpClient{
		conn:    conn,
		timeout: c.Timeout.Duration(),
		client:  &control.NTPClient{Connection: conn},
	}

	return client, nil
}

type ntpClient struct {
	conn    net.Conn
	timeout time.Duration
	client  *control.NTPClient
}

func (c *ntpClient) systemInfo() (map[string]string, error) {
	return c.peerInfo(0)
}

func (c *ntpClient) peerInfo(id uint16) (map[string]string, error) {
	msg := &control.NTPControlMsgHead{
		VnMode:        control.MakeVnMode(2, control.Mode),
		REMOp:         control.OpReadVariables,
		AssociationID: id,
	}

	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	resp, err := c.client.Communicate(msg)
	if err != nil {
		return nil, err
	}

	return resp.GetAssociationInfo()
}

func (c *ntpClient) peerIDs() ([]uint16, error) {
	msg := &control.NTPControlMsgHead{
		VnMode: control.MakeVnMode(2, control.Mode),
		REMOp:  control.OpReadStatus,
	}

	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	resp, err := c.client.Communicate(msg)
	if err != nil {
		return nil, err
	}

	peers, err := resp.GetAssociations()
	if err != nil {
		return nil, err
	}

	var ids []uint16
	for id := range peers {
		ids = append(ids, id)
	}

	return ids, nil
}

func (c *ntpClient) close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}
