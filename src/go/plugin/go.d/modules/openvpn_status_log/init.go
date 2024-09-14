// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
)

func (o *OpenVPNStatusLog) validateConfig() error {
	if o.LogPath == "" {
		return errors.New("empty 'log_path'")
	}
	return nil
}

func (o *OpenVPNStatusLog) initPerUserStatsMatcher() (matcher.Matcher, error) {
	if o.PerUserStats.Empty() {
		return nil, nil
	}
	m, err := o.PerUserStats.Parse()
	if err != nil {
		return nil, err
	}

	return m, nil
}
