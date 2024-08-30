// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq_dhcp

import (
	_ "embed"
	"errors"
	"net"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("dnsmasq_dhcp", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *DnsmasqDHCP {
	return &DnsmasqDHCP{
		Config: Config{
			// debian defaults
			LeasesPath: "/var/lib/misc/dnsmasq.leases",
			ConfPath:   "/etc/dnsmasq.conf",
			ConfDir:    "/etc/dnsmasq.d,.dpkg-dist,.dpkg-old,.dpkg-new",
		},
		charts:           charts.Copy(),
		parseConfigEvery: time.Minute,
		cacheDHCPRanges:  make(map[string]bool),
		mx:               make(map[string]int64),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	LeasesPath  string `yaml:"leases_path" json:"leases_path"`
	ConfPath    string `yaml:"conf_path,omitempty" json:"conf_path"`
	ConfDir     string `yaml:"conf_dir,omitempty" json:"conf_dir"`
}

type DnsmasqDHCP struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	leasesModTime    time.Time
	parseConfigTime  time.Time
	parseConfigEvery time.Duration
	dhcpRanges       []iprange.Range
	dhcpHosts        []net.IP
	cacheDHCPRanges  map[string]bool

	mx map[string]int64
}

func (d *DnsmasqDHCP) Configuration() any {
	return d.Config
}

func (d *DnsmasqDHCP) Init() error {
	if err := d.validateConfig(); err != nil {
		d.Errorf("config validation: %v", err)
		return err
	}
	if err := d.checkLeasesPath(); err != nil {
		d.Errorf("leases path check: %v", err)
		return err
	}

	return nil
}

func (d *DnsmasqDHCP) Check() error {
	mx, err := d.collect()
	if err != nil {
		d.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
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
