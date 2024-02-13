// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"fmt"
	"time"
)

const scaleFactor = 1000000000

func (c *Chrony) collect() (map[string]int64, error) {
	if c.client == nil {
		client, err := c.newClient(c.Config)
		if err != nil {
			return nil, err
		}
		c.client = client
	}

	mx := make(map[string]int64)

	if err := c.collectTracking(mx); err != nil {
		return nil, err
	}
	if err := c.collectActivity(mx); err != nil {
		return mx, err
	}

	return mx, nil
}

const (
	// https://github.com/mlichvar/chrony/blob/7daf34675a5a2487895c74d1578241ca91a4eb70/ntp.h#L70-L75
	leapStatusNormal         = 0
	leapStatusInsertSecond   = 1
	leapStatusDeleteSecond   = 2
	leapStatusUnsynchronised = 3
)

func (c *Chrony) collectTracking(mx map[string]int64) error {
	reply, err := c.client.Tracking()
	if err != nil {
		return fmt.Errorf("error on collecting tracking: %v", err)
	}

	mx["stratum"] = int64(reply.Stratum)
	mx["leap_status_normal"] = boolToInt(reply.LeapStatus == leapStatusNormal)
	mx["leap_status_insert_second"] = boolToInt(reply.LeapStatus == leapStatusInsertSecond)
	mx["leap_status_delete_second"] = boolToInt(reply.LeapStatus == leapStatusDeleteSecond)
	mx["leap_status_unsynchronised"] = boolToInt(reply.LeapStatus == leapStatusUnsynchronised)
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

func (c *Chrony) collectActivity(mx map[string]int64) error {
	reply, err := c.client.Activity()
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

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
