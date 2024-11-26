// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"fmt"
	"time"
)

func (c *Collector) collect() (map[string]int64, error) {
	var err error

	if err := c.client.Connect(); err != nil {
		return nil, err
	}
	defer func() { _ = c.client.Disconnect() }()

	mx := make(map[string]int64)

	if err = c.collectLoadStats(mx); err != nil {
		return nil, err
	}

	if c.perUserMatcher != nil {
		if err = c.collectUsers(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (c *Collector) collectLoadStats(mx map[string]int64) error {
	stats, err := c.client.LoadStats()
	if err != nil {
		return err
	}

	mx["clients"] = stats.NumOfClients
	mx["bytes_in"] = stats.BytesIn
	mx["bytes_out"] = stats.BytesOut
	return nil
}

func (c *Collector) collectUsers(mx map[string]int64) error {
	users, err := c.client.Users()
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

		if !c.perUserMatcher.MatchString(name) {
			continue
		}
		if !c.collectedUsers[name] {
			c.collectedUsers[name] = true
			if err := c.addUserCharts(name); err != nil {
				c.Warning(err)
			}
		}
		mx[name+"_bytes_received"] = user.BytesReceived
		mx[name+"_bytes_sent"] = user.BytesSent
		mx[name+"_connection_time"] = now - user.ConnectedSince
	}
	return nil
}

func (c *Collector) addUserCharts(userName string) error {
	cs := userCharts.Copy()

	for _, chart := range *cs {
		chart.ID = fmt.Sprintf(chart.ID, userName)
		chart.Fam = fmt.Sprintf(chart.Fam, userName)

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, userName)
		}
		chart.MarkNotCreated()
	}
	return c.charts.Add(*cs...)
}
