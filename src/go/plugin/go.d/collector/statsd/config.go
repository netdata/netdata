// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	UpdateEvery       int              `yaml:"update_every,omitempty" json:"update_every"`
	Listeners         []ListenerConfig `yaml:"listeners"              json:"listeners"`
	Profiles          []string         `yaml:"profiles,omitempty"     json:"profiles"`
	MaxSeries         int              `yaml:"max_series"             json:"max_series"`
	MetricIdleTimeout confopt.Duration `yaml:"metric_idle_timeout"    json:"metric_idle_timeout"`
	MaxTCPConnections int              `yaml:"max_tcp_connections"    json:"max_tcp_connections"`
}

// ListenerConfig is one required endpoint. Address is host:port; every
// configured listener must bind for the job to start.
type ListenerConfig struct {
	Protocol string `yaml:"protocol" json:"protocol"`
	Address  string `yaml:"address"  json:"address"`
}

func validateListeners(listeners []ListenerConfig) error {
	if len(listeners) == 0 {
		return errors.New("at least one listener is required")
	}
	seen := make(map[ListenerConfig]bool, len(listeners))
	for i, l := range listeners {
		if l.Protocol != protocolUDP && l.Protocol != protocolTCP {
			return fmt.Errorf("listeners[%d]: protocol must be %q or %q", i, protocolUDP, protocolTCP)
		}
		_, port, err := net.SplitHostPort(l.Address)
		if err != nil {
			return fmt.Errorf("listeners[%d]: address: %w", i, err)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("listeners[%d]: port must be 1-65535", i)
		}
		if seen[l] {
			return fmt.Errorf("listeners[%d]: duplicate listener", i)
		}
		seen[l] = true
	}
	return nil
}
