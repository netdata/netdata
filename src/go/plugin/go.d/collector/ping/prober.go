// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	probing "github.com/prometheus-community/pro-bing"
)

type Prober interface {
	Ping(host string) (*probing.Statistics, error)
}

func NewProber(conf ProberConfig, log *logger.Logger) Prober {
	return &pingProber{
		conf:   conf,
		Logger: log,
	}
}

type ProberConfig struct {
	Network    string           `yaml:"network,omitempty" json:"network"`
	Interface  string           `yaml:"interface,omitempty" json:"interface"`
	Privileged bool             `yaml:"privileged" json:"privileged"`
	Packets    int              `yaml:"packets,omitempty" json:"packets"`
	Interval   confopt.Duration `yaml:"interval,omitempty" json:"interval"`
	Timeout    time.Duration    `yaml:"-,omitempty" json:",omitempty"`
}

type pingProber struct {
	*logger.Logger

	conf ProberConfig
}

func (p *pingProber) Ping(host string) (*probing.Statistics, error) {
	pr := probing.New(host)

	pr.SetNetwork(p.conf.Network)

	if err := pr.Resolve(); err != nil {
		return nil, fmt.Errorf("DNS lookup '%s' : %v", host, err)
	}

	pr.RecordRtts = false
	pr.RecordTTLs = false
	pr.Interval = p.conf.Interval.Duration()
	pr.Count = p.conf.Packets
	pr.Timeout = p.conf.Timeout
	pr.InterfaceName = p.conf.Interface
	pr.SetPrivileged(p.conf.Privileged)
	pr.SetLogger(nil)

	if err := pr.Run(); err != nil {
		return nil, fmt.Errorf("pinging host '%s' (ip '%s' iface '%s'): %v",
			pr.Addr(), pr.IPAddr(), pr.InterfaceName, err)
	}

	stats := pr.Statistics()

	p.Debugf("ping stats for host '%s' (ip '%s'): %+v", pr.Addr(), pr.IPAddr(), stats)

	return stats, nil
}
