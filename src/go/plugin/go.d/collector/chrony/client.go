// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"fmt"
	"net"
	"time"

	"github.com/facebook/time/ntp/chrony"
)

type chronyConn interface {
	tracking() (*chrony.ReplyTracking, error)
	activity() (*chrony.ReplyActivity, error)
	close()
}

func newChronyConn(cfg Config) (chronyConn, error) {
	conn, err := net.DialTimeout("udp", cfg.Address, cfg.Timeout.Duration())
	if err != nil {
		return nil, err
	}

	client := &chronyClient{
		conn: conn,
		client: &chrony.Client{
			Connection: &connWithTimeout{
				Conn:    conn,
				timeout: cfg.Timeout.Duration(),
			},
		},
	}

	return client, nil
}

type chronyClient struct {
	conn   net.Conn
	client *chrony.Client
}

func (c *chronyClient) tracking() (*chrony.ReplyTracking, error) {
	req := chrony.NewTrackingPacket()

	reply, err := c.client.Communicate(req)
	if err != nil {
		return nil, err
	}

	tracking, ok := reply.(*chrony.ReplyTracking)
	if !ok {
		return nil, fmt.Errorf("unexpected reply type, want=%T, got=%T", &chrony.ReplyTracking{}, reply)
	}

	return tracking, nil
}

func (c *chronyClient) activity() (*chrony.ReplyActivity, error) {
	req := chrony.NewActivityPacket()

	reply, err := c.client.Communicate(req)
	if err != nil {
		return nil, err
	}

	activity, ok := reply.(*chrony.ReplyActivity)
	if !ok {
		return nil, fmt.Errorf("unexpected reply type, want=%T, got=%T", &chrony.ReplyActivity{}, reply)
	}

	return activity, nil
}

func (c *chronyClient) close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

type connWithTimeout struct {
	net.Conn
	timeout time.Duration
}

func (c *connWithTimeout) Read(p []byte) (n int, err error) {
	if err := c.Conn.SetReadDeadline(c.deadline()); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *connWithTimeout) Write(p []byte) (n int, err error) {
	if err := c.Conn.SetWriteDeadline(c.deadline()); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}

func (c *connWithTimeout) deadline() time.Time {
	return time.Now().Add(c.timeout)
}
