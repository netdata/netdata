// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

func (c *Collector) validateConfig() error {
	if c.LogPath == "" {
		return errors.New("empty 'log_path'")
	}
	return nil
}

func (c *Collector) initPerUserStatsMatcher() (matcher.Matcher, error) {
	if c.PerUserStats.Empty() {
		return nil, nil
	}
	m, err := c.PerUserStats.Parse()
	if err != nil {
		return nil, err
	}

	return m, nil
}
