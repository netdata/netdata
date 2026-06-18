// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

//go:embed "config_schema.json"
var configSchema string

// Register registers the SNMP collector with its shared SNMP-family device store.
func Register(store *ddsnmp.DeviceStore) {
	collectorapi.Register("snmp", newCreator(store))
}

func newCreator(store *ddsnmp.DeviceStore) collectorapi.Creator {
	if store == nil {
		panic("snmp Register requires a non-nil device store")
	}
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create:        func() collectorapi.CollectorV1 { return New(store) },
		Config:        func() any { return &Config{} },
		Methods:       snmpMethods,
		MethodHandler: snmpFunctionHandler,
	}
}

// New returns an SNMP collector. A nil store creates an isolated store for direct use.
func New(store *ddsnmp.DeviceStore) *Collector {
	if store == nil {
		store = ddsnmp.NewDeviceStore()
	}
	c := &Collector{
		Config: Config{
			CreateVnode:              true,
			VnodeDeviceDownThreshold: 3,
			Community:                "public",
			Options: OptionsConfig{
				Port:           161,
				Retries:        1,
				Timeout:        5,
				Version:        gosnmp.Version2c.String(),
				MaxOIDs:        20,
				MaxRepetitions: 25,
			},
			User: UserConfig{
				SecurityLevel: "authPriv",
				AuthProto:     "sha512",
				PrivProto:     "aes192c",
			},
			Ping: PingConfig{
				Enabled: true,
				ProbeConfig: pinger.ProbeConfig{
					Privileged: true,
					Packets:    3,
					Interval:   confopt.Duration(time.Millisecond * 100),
					Network:    "ip",
				},
			},
		},

		charts:            &collectorapi.Charts{},
		seenScalarMetrics: make(map[string]bool),
		seenTableMetrics:  make(map[string]bool),
		seenProfiles:      make(map[string]bool),
		deviceStore:       store,

		ifaceCache: newIfaceCache(),
		licensing:  newLicensingIntegration(),

		newPinger:     pinger.New,
		newSnmpClient: gosnmp.NewHandler,
		newDdSnmpColl: func(cfg ddsnmpcollector.Config) ddCollector {
			return ddsnmpcollector.New(cfg)
		},
	}

	c.funcRouter = newFuncRouter(c.ifaceCache)
	c.licensing.registerFunction(c.funcRouter)

	return c
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		vnode *vnodes.VirtualNode

		charts            *collectorapi.Charts
		seenScalarMetrics map[string]bool
		seenTableMetrics  map[string]bool
		seenProfiles      map[string]bool
		deviceStore       *ddsnmp.DeviceStore

		ifaceCache *ifaceCache // interface metrics cache for functions
		licensing  *licensingIntegration
		bgp        *bgpIntegration // BGP metric normalization and function state
		funcRouter *funcRouter     // function router for method handlers

		pingClient pinger.Client
		newPinger  func(pinger.Config, *logger.Logger) (pinger.Client, error)

		snmpClient    gosnmp.Handler
		newSnmpClient func() gosnmp.Handler

		ddSnmpColl    ddCollector
		newDdSnmpColl func(ddsnmpcollector.Config) ddCollector

		sysInfo      *snmputils.SysInfo
		snmpProfiles []*ddsnmp.Profile

		adjMaxRepetitions uint32

		disableBulkWalk bool
	}
	ddCollector interface {
		Collect() ([]*ddsnmp.ProfileMetrics, error)
		CollectDeviceMetadata() (map[string]ddsnmp.MetaTag, error)
	}
)

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

	if c.PingOnly || c.Ping.Enabled {
		pr, err := c.initPinger()
		if err != nil {
			return fmt.Errorf("failed to initialize ping client: %v", err)
		}
		c.pingClient = pr
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
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

	if c.PingOnly && c.pingClient != nil {
		if _, err := c.pingClient.Probe(ctx, c.Hostname); err != nil && isPingUnrecoverableError(err) {
			return fmt.Errorf("ping check failed: %v", err)
		}
	}

	return nil
}

func (c *Collector) Charts() *collectorapi.Charts {
	return c.charts
}

func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx, err := c.collect(ctx)
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.funcRouter != nil {
		c.funcRouter.Cleanup(ctx)
	}
	if c.deviceStore != nil {
		c.deviceStore.Unregister(c.deviceStoreKey())
	}
	if c.snmpClient != nil {
		_ = c.snmpClient.Close()
	}
}

func (c *Collector) VirtualNode() *vnodes.VirtualNode {
	return c.vnode
}
