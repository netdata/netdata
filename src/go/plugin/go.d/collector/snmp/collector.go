// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

	"github.com/gosnmp/gosnmp"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("snmp", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			CreateVnode: true,
			Community:   "public",
			Options: Options{
				Port:           161,
				Retries:        1,
				Timeout:        5,
				Version:        gosnmp.Version2c.String(),
				MaxOIDs:        60,
				MaxRepetitions: 25,
			},
			User: User{
				SecurityLevel: "authPriv",
				AuthProto:     "sha512",
				PrivProto:     "aes192c",
			},
		},

		newSnmpClient: gosnmp.NewHandler,

		checkMaxReps:  true,
		collectIfMib:  true,
		netInterfaces: make(map[string]*netInterface),
	}
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	vnode *vnodes.VirtualNode

	charts *module.Charts

	newSnmpClient func() gosnmp.Handler
	snmpClient    gosnmp.Handler

	netIfaceFilterByName matcher.Matcher
	netIfaceFilterByType matcher.Matcher

	checkMaxReps bool
	collectIfMib bool

	netInterfaces map[string]*netInterface

	sysInfo *snmpsd.SysInfo

	customOids []string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}

	snmpClient, err := c.initSNMPClient()
	if err != nil {
		return fmt.Errorf("failed to initialize SNMP client: %v", err)
	}

	err = snmpClient.Connect()
	if err != nil {
		return fmt.Errorf("SNMP client connection failed: %v", err)
	}
	c.snmpClient = snmpClient

	byName, byType, err := c.initNetIfaceFilters()
	if err != nil {
		return fmt.Errorf("failed to initialize network interface filters: %v", err)
	}
	c.netIfaceFilterByName = byName
	c.netIfaceFilterByType = byType

	charts, err := newUserInputCharts(c.ChartsInput)
	if err != nil {
		return fmt.Errorf("failed to create user charts: %v", err)
	}
	c.charts = charts

	c.customOids = c.initOIDs()

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

func (c *Collector) Cleanup(context.Context) {
	if c.snmpClient != nil {
		_ = c.snmpClient.Close()
	}
}

func (c *Collector) VirtualNode() *vnodes.VirtualNode {
	return c.vnode
}
