// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
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
			CreateVnode:                true,
			EnableProfiles:             true,
			EnableProfilesTableMetrics: true,
			VnodeDeviceDownThreshold:   3,
			Community:                  "public",
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

		charts:            &module.Charts{},
		seenScalarMetrics: make(map[string]bool),
		seenTableMetrics:  make(map[string]bool),

		newSnmpClient: gosnmp.NewHandler,

		snmpBulkWalkOk: true,
		netInterfaces:  make(map[string]*netInterface),
		collectIfMib:   true,
	}
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	vnode *vnodes.VirtualNode

	charts            *module.Charts
	seenScalarMetrics map[string]bool
	seenTableMetrics  map[string]bool

	newSnmpClient func() gosnmp.Handler
	snmpClient    gosnmp.Handler
	ddSnmpColl    *ddsnmpcollector.Collector

	sysInfo      *snmputils.SysInfo
	snmpProfiles []*ddsnmp.Profile

	adjMaxRepetitions uint32
	snmpBulkWalkOk    bool

	// legacy data collection parameters
	netIfaceFilterByName matcher.Matcher
	netIfaceFilterByType matcher.Matcher
	collectIfMib         bool // only for tests
	netInterfaces        map[string]*netInterface
	customOids           []string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %v", err)
	}

	if _, err := c.initSNMPClient(); err != nil {
		return fmt.Errorf("failed to initialize SNMP client: %v", err)
	}

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
	if c.snmpClient == nil {
		snmpClient, err := c.initAndConnectSNMPClient()
		if err != nil {
			return fmt.Errorf("failed to init and connect SNMP client: %v", err)
		}
		c.snmpClient = snmpClient
	}

	if _, err := snmputils.GetSysInfo(c.snmpClient); err != nil {
		return err
	}

	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		// Some buggy SNMPv3 devices occasionally get stuck with
		// "packet is not authentic" errors. Closing and dropping
		// the client here forces a reconnect on the next scrape,
		// which usually recovers the session.
		if strings.Contains(err.Error(), "packet is not authentic") {
			c.Cleanup(ctx)
			c.snmpClient = nil
		}
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
