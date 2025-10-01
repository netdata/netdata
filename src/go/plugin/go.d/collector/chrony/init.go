// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"errors"
	"net"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		return errors.New("empty 'address'")
	}
	return nil
}

func (c *Collector) initChronycBinary() (chronyBinary, error) {
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil {
		return nil, err
	}

	// 'serverstats' allowed only through the Unix domain socket
	if !isLocalhost(host) {
		return nil, nil
	}

	chronyc := newChronycExec(c.Timeout.Duration(), c.Logger)

	return chronyc, nil
}

func isLocalhost(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
