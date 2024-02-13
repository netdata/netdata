// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"fmt"
	"net"

	"github.com/facebook/time/ntp/chrony"
)

func newChronyClient(c Config) (chronyClient, error) {
	conn, err := net.DialTimeout("udp", c.Address, c.Timeout.Duration)
	if err != nil {
		return nil, err
	}

	client := &simpleClient{
		conn:   conn,
		client: &chrony.Client{Connection: conn},
	}
	return client, nil
}

type simpleClient struct {
	conn   net.Conn
	client *chrony.Client
}

func (sc *simpleClient) Tracking() (*chrony.ReplyTracking, error) {
	reply, err := sc.client.Communicate(chrony.NewTrackingPacket())
	if err != nil {
		return nil, err
	}

	tracking, ok := reply.(*chrony.ReplyTracking)
	if !ok {
		return nil, fmt.Errorf("unexpected reply type, want=%T, got=%T", &chrony.ReplyTracking{}, reply)
	}
	return tracking, nil
}

func (sc *simpleClient) Activity() (*chrony.ReplyActivity, error) {
	reply, err := sc.client.Communicate(chrony.NewActivityPacket())
	if err != nil {
		return nil, err
	}

	activity, ok := reply.(*chrony.ReplyActivity)
	if !ok {
		return nil, fmt.Errorf("unexpected reply type, want=%T, got=%T", &chrony.ReplyActivity{}, reply)
	}
	return activity, nil
}

func (sc *simpleClient) Close() {
	if sc.conn != nil {
		_ = sc.conn.Close()
		sc.conn = nil
	}
}
