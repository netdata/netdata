// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const scaleFactor = 1000000000

const (
	// https://github.com/mlichvar/chrony/blob/7daf34675a5a2487895c74d1578241ca91a4eb70/ntp.h#L70-L75
	leapStatusNormal         = 0
	leapStatusInsertSecond   = 1
	leapStatusDeleteSecond   = 2
	leapStatusUnsynchronised = 3
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.conn == nil {
		client, err := c.newConn(c.Config)
		if err != nil {
			return nil, err
		}
		c.conn = client
	}

	mx := make(map[string]int64)

	if err := c.collectTracking(mx); err != nil {
		return nil, err
	}
	if err := c.collectActivity(mx); err != nil {
		return mx, err
	}
	if c.exec != nil {
		if err := c.collectServerStats(mx); err != nil {
			c.Warning(err)
			c.exec = nil
		} else {
			c.addServerStatsChartsOnce.Do(c.addServerStatsCharts)
		}
	}

	return mx, nil
}

func (c *Collector) collectTracking(mx map[string]int64) error {
	reply, err := c.conn.tracking()
	if err != nil {
		return fmt.Errorf("error on collecting tracking: %v", err)
	}

	mx["stratum"] = int64(reply.Stratum)
	mx["leap_status_normal"] = metrix.Bool(reply.LeapStatus == leapStatusNormal)
	mx["leap_status_insert_second"] = metrix.Bool(reply.LeapStatus == leapStatusInsertSecond)
	mx["leap_status_delete_second"] = metrix.Bool(reply.LeapStatus == leapStatusDeleteSecond)
	mx["leap_status_unsynchronised"] = metrix.Bool(reply.LeapStatus == leapStatusUnsynchronised)
	mx["root_delay"] = int64(reply.RootDelay * scaleFactor)
	mx["root_dispersion"] = int64(reply.RootDispersion * scaleFactor)
	mx["skew"] = int64(reply.SkewPPM * scaleFactor)
	mx["last_offset"] = int64(reply.LastOffset * scaleFactor)
	mx["rms_offset"] = int64(reply.RMSOffset * scaleFactor)
	mx["update_interval"] = int64(reply.LastUpdateInterval * scaleFactor)
	// handle chrony restarts
	if reply.RefTime.Year() != 1970 {
		mx["ref_measurement_time"] = time.Now().Unix() - reply.RefTime.Unix()
	}
	mx["residual_frequency"] = int64(reply.ResidFreqPPM * scaleFactor)
	// https://github.com/mlichvar/chrony/blob/5b04f3ca902e5d10aa5948fb7587d30b43941049/client.c#L1706
	mx["current_correction"] = abs(int64(reply.CurrentCorrection * scaleFactor))
	mx["frequency"] = abs(int64(reply.FreqPPM * scaleFactor))

	return nil
}

func (c *Collector) collectActivity(mx map[string]int64) error {
	reply, err := c.conn.activity()
	if err != nil {
		return fmt.Errorf("error on collecting activity: %v", err)
	}

	mx["online_sources"] = int64(reply.Online)
	mx["offline_sources"] = int64(reply.Offline)
	mx["burst_online_sources"] = int64(reply.BurstOnline)
	mx["burst_offline_sources"] = int64(reply.BurstOffline)
	mx["unresolved_sources"] = int64(reply.Unresolved)

	return nil
}

func (c *Collector) collectServerStats(mx map[string]int64) error {
	bs, err := c.exec.serverStats()
	if err != nil {
		return fmt.Errorf("error on collecting server stats: %v", err)
	}

	sc := bufio.NewScanner(bytes.NewReader(bs))
	var n int

	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}

		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		switch key {
		case "NTP packets received",
			"NTP packets dropped",
			"Command packets received",
			"Command packets dropped":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				key = strings.ToLower(strings.ReplaceAll(key, " ", "_"))
				mx[key] = v
				n++
			}
		}
	}

	if n == 0 {
		return errors.New("no server stats metrics found in the response")
	}

	return nil
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
