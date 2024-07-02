// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"fmt"
	"time"
)

func (o *OpenVPN) collect() (map[string]int64, error) {
	var err error

	if err := o.client.Connect(); err != nil {
		return nil, err
	}
	defer func() { _ = o.client.Disconnect() }()

	mx := make(map[string]int64)

	if err = o.collectLoadStats(mx); err != nil {
		return nil, err
	}

	if o.perUserMatcher != nil {
		if err = o.collectUsers(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (o *OpenVPN) collectLoadStats(mx map[string]int64) error {
	stats, err := o.client.LoadStats()
	if err != nil {
		return err
	}

	mx["clients"] = stats.NumOfClients
	mx["bytes_in"] = stats.BytesIn
	mx["bytes_out"] = stats.BytesOut
	return nil
}

func (o *OpenVPN) collectUsers(mx map[string]int64) error {
	users, err := o.client.Users()
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	var name string

	for _, user := range users {
		if user.Username == "UNDEF" {
			name = user.CommonName
		} else {
			name = user.Username
		}

		if !o.perUserMatcher.MatchString(name) {
			continue
		}
		if !o.collectedUsers[name] {
			o.collectedUsers[name] = true
			if err := o.addUserCharts(name); err != nil {
				o.Warning(err)
			}
		}
		mx[name+"_bytes_received"] = user.BytesReceived
		mx[name+"_bytes_sent"] = user.BytesSent
		mx[name+"_connection_time"] = now - user.ConnectedSince
	}
	return nil
}

func (o *OpenVPN) addUserCharts(userName string) error {
	cs := userCharts.Copy()

	for _, chart := range *cs {
		chart.ID = fmt.Sprintf(chart.ID, userName)
		chart.Fam = fmt.Sprintf(chart.Fam, userName)

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, userName)
		}
		chart.MarkNotCreated()
	}
	return o.charts.Add(*cs...)
}
