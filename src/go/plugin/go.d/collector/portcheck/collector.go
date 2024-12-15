// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
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

func New() *Collector {
	return &Collector{
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
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Host        string           `yaml:"host" json:"host"`
	Ports       []int            `yaml:"ports" json:"ports"`
	UDPPorts    []int            `yaml:"udp_ports,omitempty" json:"udp_ports"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	c.tcpPorts, c.udpPorts = c.initPorts()

	c.Debugf("using host: %s", c.Host)
	c.Debugf("using ports: tcp %v udp %v", c.Ports, c.UDPPorts)
	c.Debugf("using connection timeout: %s", c.Timeout)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {}
