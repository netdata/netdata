// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	probing "github.com/prometheus-community/pro-bing"
)

type prober interface {
	ping(host string) (*probing.Statistics, error)
}

func newPingProber(conf pingProberConfig, log *logger.Logger) prober {
	return &pingProber{
		network:       conf.network,
		interfaceName: conf.ifaceName,
		privileged:    conf.privileged,
		packets:       conf.packets,
		interval:      conf.interval,
		deadline:      conf.deadline,
		Logger:        log,
	}
}

type pingProberConfig struct {
	network    string
	ifaceName  string
	privileged bool
	packets    int
	interval   time.Duration
	deadline   time.Duration
}

type pingProber struct {
	*logger.Logger

	network       string
	interfaceName string
	privileged    bool
	packets       int
	interval      time.Duration
	deadline      time.Duration
}

func (p *pingProber) ping(host string) (*probing.Statistics, error) {
	pr := probing.New(host)

	pr.SetNetwork(p.network)

	if err := pr.Resolve(); err != nil {
		return nil, fmt.Errorf("DNS lookup '%s' : %v", host, err)
	}

	pr.RecordRtts = false
	pr.Interval = p.interval
	pr.Count = p.packets
	pr.Timeout = p.deadline
	pr.InterfaceName = p.interfaceName
	pr.SetPrivileged(p.privileged)
	pr.SetLogger(nil)

	if err := pr.Run(); err != nil {
		return nil, fmt.Errorf("pinging host '%s' (ip '%s' iface '%s'): %v",
			pr.Addr(), pr.IPAddr(), pr.InterfaceName, err)
	}

	stats := pr.Statistics()

	p.Debugf("ping stats for host '%s' (ip '%s'): %+v", pr.Addr(), pr.IPAddr(), stats)

	return stats, nil
}
