// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	_ "embed"
	"errors"
	"net"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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
		Config: func() any { return &Config{} },
	})
}

func New() *PortCheck {
	return &PortCheck{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts: &module.Charts{},

		dialTCP: net.DialTimeout,

		scanUDP:    scanUDPPort,
		doUdpPorts: true,

		seenUdpPorts: make(map[int]bool),
		seenTcpPorts: make(map[int]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Host        string           `yaml:"host" json:"host"`
	Ports       []int            `yaml:"ports" json:"ports"`
	UDPPorts    []int            `yaml:"udp_ports,omitempty" json:"udp_ports"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type PortCheck struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	dialTCP dialTCPFunc
	scanUDP func(address string, timeout time.Duration) (bool, error)

	tcpPorts     []*tcpPort
	seenTcpPorts map[int]bool

	udpPorts     []*udpPort
	seenUdpPorts map[int]bool
	doUdpPorts   bool
}

func (pc *PortCheck) Configuration() any {
	return pc.Config
}

func (pc *PortCheck) Init() error {
	if err := pc.validateConfig(); err != nil {
		pc.Errorf("config validation: %v", err)
		return err
	}

	pc.tcpPorts, pc.udpPorts = pc.initPorts()

	pc.Debugf("using host: %s", pc.Host)
	pc.Debugf("using ports: tcp %v udp %v", pc.Ports, pc.UDPPorts)
	pc.Debugf("using connection timeout: %s", pc.Timeout)

	return nil
}

func (pc *PortCheck) Check() error {
	mx, err := pc.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
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
