// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq_dhcp

import (
	_ "embed"
	"net"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/iprange"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dnsmasq_dhcp", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *DnsmasqDHCP {
	config := Config{
		// debian defaults
		LeasesPath: "/var/lib/misc/dnsmasq.leases",
		ConfPath:   "/etc/dnsmasq.conf",
		ConfDir:    "/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new",
	}

	return &DnsmasqDHCP{
		Config:           config,
		charts:           charts.Copy(),
		parseConfigEvery: time.Minute,
		cacheDHCPRanges:  make(map[string]bool),
		mx:               make(map[string]int64),
	}
}

type Config struct {
	LeasesPath string `yaml:"leases_path"`
	ConfPath   string `yaml:"conf_path"`
	ConfDir    string `yaml:"conf_dir"`
}

type DnsmasqDHCP struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	leasesModTime time.Time

	parseConfigTime  time.Time
	parseConfigEvery time.Duration

	dhcpRanges []iprange.Range
	dhcpHosts  []net.IP

	cacheDHCPRanges map[string]bool

	mx map[string]int64
}

func (d *DnsmasqDHCP) Init() bool {
	if err := d.validateConfig(); err != nil {
		d.Errorf("config validation: %v", err)
		return false
	}
	if err := d.checkLeasesPath(); err != nil {
		d.Errorf("leases path check: %v", err)
		return false
	}

	return true
}

func (d *DnsmasqDHCP) Check() bool {
	return len(d.Collect()) > 0
}

func (d *DnsmasqDHCP) Charts() *module.Charts {
	return d.charts
}

func (d *DnsmasqDHCP) Collect() map[string]int64 {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (d *DnsmasqDHCP) Cleanup() {}
