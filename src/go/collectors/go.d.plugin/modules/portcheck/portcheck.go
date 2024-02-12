// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	_ "embed"
	"net"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("portcheck", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *PortCheck {
	return &PortCheck{
		Config: Config{
			Timeout: web.Duration{Duration: time.Second * 2},
		},
		dial: net.DialTimeout,
	}
}

type Config struct {
	Host    string       `yaml:"host"`
	Ports   []int        `yaml:"ports"`
	Timeout web.Duration `yaml:"timeout"`
}

type dialFunc func(network, address string, timeout time.Duration) (net.Conn, error)

type port struct {
	number  int
	state   checkState
	inState int
	latency int
}

type PortCheck struct {
	module.Base
	Config      `yaml:",inline"`
	UpdateEvery int `yaml:"update_every"`

	charts *module.Charts
	dial   dialFunc
	ports  []*port
}

func (pc *PortCheck) Init() bool {
	if err := pc.validateConfig(); err != nil {
		pc.Errorf("config validation: %v", err)
		return false
	}

	charts, err := pc.initCharts()
	if err != nil {
		pc.Errorf("init charts: %v", err)
		return false
	}
	pc.charts = charts

	for _, p := range pc.Ports {
		pc.ports = append(pc.ports, &port{number: p})
	}

	pc.Debugf("using host: %s", pc.Host)
	pc.Debugf("using ports: %v", pc.Ports)
	pc.Debugf("using TCP connection timeout: %s", pc.Timeout)

	return true
}

func (pc *PortCheck) Check() bool {
	return true
}

func (pc *PortCheck) Charts() *module.Charts {
	return pc.charts
}

func (pc *PortCheck) Collect() map[string]int64 {
	mx, err := pc.collect()
	if err != nil {
		pc.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (pc *PortCheck) Cleanup() {}
