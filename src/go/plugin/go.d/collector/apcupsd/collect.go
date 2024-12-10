// SPDX-License-Identifier: GPL-3.0-or-later

package apcupsd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const precision = 100

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		conn, err := c.establishConnection()
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	resp, err := c.conn.status()
	if err != nil {
		c.Cleanup(context.Background())
		return nil, err
	}

	mx := make(map[string]int64)

	if err := c.collectStatus(mx, resp); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectStatus(mx map[string]int64, resp []byte) error {
	st, err := parseStatus(resp)
	if err != nil {
		return fmt.Errorf("failed to parse status: %v", err)
	}

	if st.status == "" {
		return errors.New("unexpected response: status is empty")
	}

	for _, v := range upsStatuses {
		mx["status_"+v] = 0
	}
	for _, v := range strings.Fields(st.status) {
		mx["status_"+v] = 1
	}

	switch st.status {
	case "COMMLOST", "SHUTTING_DOWN":
		return nil
	}

	if st.selftest != "" {
		for _, v := range upsSelftestStatuses {
			mx["selftest_"+v] = 0
		}
		mx["selftest_"+st.selftest] = 1
	}

	if st.bcharge != nil {
		mx["battery_charge"] = int64(*st.bcharge * precision)
	}
	if st.battv != nil {
		mx["battery_voltage"] = int64(*st.battv * precision)
	}
	if st.nombattv != nil {
		mx["battery_voltage_nominal"] = int64(*st.nombattv * precision)
	}
	if st.linev != nil {
		mx["input_voltage"] = int64(*st.linev * precision)
	}
	if st.minlinev != nil {
		mx["input_voltage_min"] = int64(*st.minlinev * precision)
	}
	if st.maxlinev != nil {
		mx["input_voltage_max"] = int64(*st.maxlinev * precision)
	}
	if st.linefreq != nil {
		mx["input_frequency"] = int64(*st.linefreq * precision)
	}
	if st.outputv != nil {
		mx["output_voltage"] = int64(*st.outputv * precision)
	}
	if st.nomoutv != nil {
		mx["output_voltage_nominal"] = int64(*st.nomoutv * precision)
	}
	if st.loadpct != nil {
		mx["load_percent"] = int64(*st.loadpct * precision)
	}
	if st.itemp != nil {
		mx["itemp"] = int64(*st.itemp * precision)
	}
	if st.timeleft != nil {
		mx["timeleft"] = int64(*st.timeleft * 60 * precision) // to seconds
	}
	if st.nompower != nil && st.loadpct != nil {
		mx["load"] = int64(*st.nompower * *st.loadpct)
	}
	if st.battdate != "" {
		if v, err := battdateSecondsAgo(st.battdate); err != nil {
			c.Debugf("failed to calculate time since battery replacement for date '%s': %v", st.battdate, err)
		} else {
			mx["battery_seconds_since_replacement"] = v
		}
	}

	return nil
}

func battdateSecondsAgo(battdate string) (int64, error) {
	var layout string

	if strings.ContainsRune(battdate, '-') {
		layout = "2006-01-02"
	} else {
		layout = "01/02/06"
	}

	date, err := time.Parse(layout, battdate)
	if err != nil {
		return 0, err
	}

	secsAgo := int64(time.Now().Sub(date).Seconds())

	return secsAgo, nil
}

func (c *Collector) establishConnection() (apcupsdConn, error) {
	conn := c.newConn(c.Config)

	if err := conn.connect(); err != nil {
		return nil, err
	}

	return conn, nil
}
