// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

// Resource defaults. The series and TCP connection caps are provisional.
const (
	defaultMaxSeries         = 1000
	defaultMetricIdleTimeout = 30 * time.Minute
	defaultMaxTCPConnections = 64
)

// Listener protocols, untyped so they serve as protocol values and plain
// strings. udp and tcp are also the server network names.
const (
	protocolUDP  = "udp"
	protocolTCP  = "tcp"
	protocolBoth = "both" // a UDP and a TCP socket on the same address
)

// protocolSpec defines the listener protocol option: empty or omitted means both.
type protocolSpec struct{}

func (protocolSpec) Values() []string { return []string{protocolUDP, protocolTCP, protocolBoth} }
func (protocolSpec) Default() string  { return protocolBoth }

type Config struct {
	UpdateEvery        int                  `yaml:"update_every,omitempty"        json:"update_every"`
	AutoDetectionRetry int                  `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Listeners          []ListenerConfig     `yaml:"listeners"                     json:"listeners"`
	Profiles           []string             `yaml:"profiles,omitempty"            json:"profiles"`
	MaxSeries          int                  `yaml:"max_series"                    json:"max_series"`
	MetricIdleTimeout  confopt.LongDuration `yaml:"metric_idle_timeout"           json:"metric_idle_timeout"`
	MaxTCPConnections  int                  `yaml:"max_tcp_connections"           json:"max_tcp_connections"`
}

// ListenerConfig is one required endpoint. Address is host:port; every
// configured listener must bind for the job to start.
type ListenerConfig struct {
	Protocol confopt.Enum[protocolSpec] `yaml:"protocol" json:"protocol"`
	Address  string                     `yaml:"address"  json:"address"`
}

// protocols returns the transports this listener binds, in binding order.
func (l ListenerConfig) protocols() []string {
	if p := l.Protocol.Normalized(); p != protocolBoth {
		return []string{string(p)}
	}
	return []string{protocolUDP, protocolTCP}
}

// validate checks everything except profiles, which Init loads and validates.
func (c Config) validate() error {
	switch {
	case c.MaxSeries <= 0:
		return errors.New("max_series must be positive")
	case c.MetricIdleTimeout < 0:
		return errors.New("metric_idle_timeout must be nonnegative")
	case c.MaxTCPConnections <= 0:
		return errors.New("max_tcp_connections must be positive")
	}
	return validateListeners(c.Listeners)
}

func validateListeners(listeners []ListenerConfig) error {
	if len(listeners) == 0 {
		return errors.New("at least one listener is required")
	}
	// Duplicates are per bound socket, so "both" overlaps "udp" and "tcp".
	type socket struct{ protocol, address string }
	seen := make(map[socket]bool, len(listeners))
	for i, l := range listeners {
		if err := l.Protocol.Validate(); err != nil {
			return fmt.Errorf("listeners[%d]: protocol %w", i, err)
		}
		_, port, err := net.SplitHostPort(l.Address)
		if err != nil {
			return fmt.Errorf("listeners[%d]: address: %w", i, err)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("listeners[%d]: port must be 1-65535", i)
		}
		for _, protocol := range l.protocols() {
			s := socket{protocol, l.Address}
			if seen[s] {
				return fmt.Errorf("listeners[%d]: duplicate %s listener", i, protocol)
			}
			seen[s] = true
		}
	}
	return nil
}

func (c Config) hasListener(protocol string) bool {
	return slices.ContainsFunc(
		c.Listeners,
		func(l ListenerConfig) bool { return slices.Contains(l.protocols(), protocol) },
	)
}
