// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"fmt"
	"net"

	"github.com/facebook/time/ntp/chrony"
)

func newChronyClient(c Config) (chronyClient, error) {
	conn, err := net.DialTimeout("udp", c.Address, c.Timeout.Duration())
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
	req := chrony.NewTrackingPacket()

	reply, err := sc.client.Communicate(req)
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
	req := chrony.NewActivityPacket()

	reply, err := sc.client.Communicate(req)
	if err != nil {
		return nil, err
	}

	activity, ok := reply.(*chrony.ReplyActivity)
	if !ok {
		return nil, fmt.Errorf("unexpected reply type, want=%T, got=%T", &chrony.ReplyActivity{}, reply)
	}
	return activity, nil
}

type serverStats struct {
	v1 *chrony.ServerStats
	v2 *chrony.ServerStats2
	v3 *chrony.ServerStats3
	v4 *chrony.ServerStats4
}

func (sc *simpleClient) ServerStats() (*serverStats, error) {
	req := chrony.NewServerStatsPacket()

	reply, err := sc.client.Communicate(req)
	if err != nil {
		return nil, err
	}

	var stats serverStats

	switch v := reply.(type) {
	case *chrony.ReplyServerStats:
		stats.v1 = &chrony.ServerStats{
			NTPHits:  v.NTPHits,
			CMDHits:  v.CMDHits,
			NTPDrops: v.NTPDrops,
			CMDDrops: v.CMDDrops,
			LogDrops: v.LogDrops,
		}
	case *chrony.ReplyServerStats2:
		stats.v2 = &chrony.ServerStats2{
			NTPHits:     v.NTPHits,
			NKEHits:     v.NKEHits,
			CMDHits:     v.CMDHits,
			NTPDrops:    v.NTPDrops,
			NKEDrops:    v.NKEDrops,
			CMDDrops:    v.CMDDrops,
			LogDrops:    v.LogDrops,
			NTPAuthHits: v.NTPAuthHits,
		}
	case *chrony.ReplyServerStats3:
		stats.v3 = &chrony.ServerStats3{
			NTPHits:            v.NTPHits,
			NKEHits:            v.NKEHits,
			CMDHits:            v.CMDHits,
			NTPDrops:           v.NTPDrops,
			NKEDrops:           v.NKEDrops,
			CMDDrops:           v.CMDDrops,
			LogDrops:           v.LogDrops,
			NTPAuthHits:        v.NTPAuthHits,
			NTPInterleavedHits: v.NTPInterleavedHits,
			NTPTimestamps:      v.NTPTimestamps,
			NTPSpanSeconds:     v.NTPSpanSeconds,
		}
	case *chrony.ReplyServerStats4:
		stats.v4 = &chrony.ServerStats4{
			NTPHits:               v.NTPHits,
			NKEHits:               v.NKEHits,
			CMDHits:               v.CMDHits,
			NTPDrops:              v.NTPDrops,
			NKEDrops:              v.NKEDrops,
			CMDDrops:              v.CMDDrops,
			LogDrops:              v.LogDrops,
			NTPAuthHits:           v.NTPAuthHits,
			NTPInterleavedHits:    v.NTPInterleavedHits,
			NTPTimestamps:         v.NTPTimestamps,
			NTPSpanSeconds:        v.NTPSpanSeconds,
			NTPDaemonRxtimestamps: v.NTPDaemonRxtimestamps,
			NTPDaemonTxtimestamps: v.NTPDaemonTxtimestamps,
			NTPKernelRxtimestamps: v.NTPKernelRxtimestamps,
			NTPKernelTxtimestamps: v.NTPKernelTxtimestamps,
			NTPHwRxTimestamps:     v.NTPHwRxTimestamps,
			NTPHwTxTimestamps:     v.NTPHwTxTimestamps,
		}
	default:
		return nil, fmt.Errorf("unexpected reply type, want=ReplyServerStats, got=%T", reply)
	}

	return &stats, nil
}

func (sc *simpleClient) Close() {
	if sc.conn != nil {
		_ = sc.conn.Close()
		sc.conn = nil
	}
}
