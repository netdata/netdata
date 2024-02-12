// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"time"
)

func (o *OpenVPNStatusLog) collect() (map[string]int64, error) {
	clients, err := parse(o.LogPath)
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	collectTotalStats(mx, clients)

	if o.perUserMatcher != nil && numOfClients(clients) > 0 {
		o.collectUsers(mx, clients)
	}

	return mx, nil
}

func collectTotalStats(mx map[string]int64, clients []clientInfo) {
	var in, out int64
	for _, c := range clients {
		in += c.bytesReceived
		out += c.bytesSent
	}
	mx["clients"] = numOfClients(clients)
	mx["bytes_in"] = in
	mx["bytes_out"] = out
}

func (o *OpenVPNStatusLog) collectUsers(mx map[string]int64, clients []clientInfo) {
	now := time.Now().Unix()

	for _, user := range clients {
		name := user.commonName
		if !o.perUserMatcher.MatchString(name) {
			continue
		}
		if !o.collectedUsers[name] {
			o.collectedUsers[name] = true
			if err := o.addUserCharts(name); err != nil {
				o.Warning(err)
			}
		}
		mx[name+"_bytes_in"] = user.bytesReceived
		mx[name+"_bytes_out"] = user.bytesSent
		mx[name+"_connection_time"] = now - user.connectedSince
	}
}

func numOfClients(clients []clientInfo) int64 {
	var num int64
	for _, v := range clients {
		if v.commonName != "" && v.commonName != "UNDEF" {
			num++
		}
	}
	return num
}
