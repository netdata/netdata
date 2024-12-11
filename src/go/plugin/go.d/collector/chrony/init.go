// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
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

	ndsudoPath := filepath.Join(executable.Directory, "ndsudo")

	if _, err := os.Stat(ndsudoPath); err != nil {
		return nil, fmt.Errorf("ndsudo executable not found: %v", err)
	}

	chronyc := newChronycExec(ndsudoPath, c.Timeout.Duration(), c.Logger)

	return chronyc, nil
}

func isLocalhost(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
