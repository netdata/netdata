// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	probing "github.com/prometheus-community/pro-bing"
)

func newPingProber(conf pingProberConfig, log *logger.Logger) prober {
	var source string
	if conf.iface != "" {
		if addr, err := getInterfaceIPAddress(conf.iface); err != nil {
			log.Warningf("error getting interface '%s' IP address: %v", conf.iface, err)
		} else {
			log.Infof("interface '%s' IP address '%s', will use it as the source", conf.iface, addr)
			source = addr
		}
	}

	return &pingProber{
		network:    conf.network,
		privileged: conf.privileged,
		packets:    conf.packets,
		source:     source,
		interval:   conf.interval,
		deadline:   conf.deadline,
		Logger:     log,
	}
}

type pingProberConfig struct {
	network    string
	privileged bool
	packets    int
	iface      string
	interval   time.Duration
	deadline   time.Duration
}

type pingProber struct {
	*logger.Logger

	network    string
	privileged bool
	packets    int
	source     string
	interval   time.Duration
	deadline   time.Duration
}

func (p *pingProber) ping(host string) (*probing.Statistics, error) {
	pr := probing.New(host)

	pr.SetNetwork(p.network)

	if err := pr.Resolve(); err != nil {
		return nil, fmt.Errorf("DNS lookup '%s' : %v", host, err)
	}

	pr.Source = p.source
	pr.RecordRtts = false
	pr.Interval = p.interval
	pr.Count = p.packets
	pr.Timeout = p.deadline
	pr.SetPrivileged(p.privileged)
	pr.SetLogger(nil)

	if err := pr.Run(); err != nil {
		return nil, fmt.Errorf("pinging host '%s' (ip %s): %v", pr.Addr(), pr.IPAddr(), err)
	}

	stats := pr.Statistics()

	p.Debugf("ping stats for host '%s' (ip '%s'): %+v", pr.Addr(), pr.IPAddr(), stats)

	return stats, nil
}

func getInterfaceIPAddress(ifaceName string) (ipaddr string, err error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", err
	}

	addresses, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	// FIXME: add IPv6 support
	var v4Addr string
	for _, addr := range addresses {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			v4Addr = ipnet.IP.To4().String()
			break
		}
	}

	if v4Addr == "" {
		return "", errors.New("ipv4 addresses not found")
	}

	return v4Addr, nil
}
