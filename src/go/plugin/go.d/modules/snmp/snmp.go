// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"

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

func New() *SNMP {
	return &SNMP{
		Config: Config{
			Community: "public",
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

type SNMP struct {
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

	sysInfo *sysInfo

	customOids []string
}

func (s *SNMP) Configuration() any {
	return s.Config
}

func (s *SNMP) Init() error {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation failed: %v", err)
		return err
	}

	snmpClient, err := s.initSNMPClient()
	if err != nil {
		s.Errorf("failed to initialize SNMP client: %v", err)
		return err
	}

	err = snmpClient.Connect()
	if err != nil {
		s.Errorf("SNMP client connection failed: %v", err)
		return err
	}
	s.snmpClient = snmpClient

	byName, byType, err := s.initNetIfaceFilters()
	if err != nil {
		s.Errorf("failed to initialize network interface filters: %v", err)
		return err
	}
	s.netIfaceFilterByName = byName
	s.netIfaceFilterByType = byType

	charts, err := newUserInputCharts(s.ChartsInput)
	if err != nil {
		s.Errorf("failed to create user charts: %v", err)
		return err
	}
	s.charts = charts

	s.customOids = s.initOIDs()

	return nil
}

func (s *SNMP) Check() error {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *SNMP) Charts() *module.Charts {
	return s.charts
}

func (s *SNMP) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *SNMP) Cleanup() {
	if s.snmpClient != nil {
		_ = s.snmpClient.Close()
	}
}

func (s *SNMP) VirtualNode() *vnodes.VirtualNode {
	return s.vnode
}
