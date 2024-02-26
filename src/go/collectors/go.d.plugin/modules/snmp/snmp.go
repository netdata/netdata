// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/gosnmp/gosnmp"
)

const (
	defaultUpdateEvery = 10
	defaultHostname    = "127.0.0.1"
	defaultCommunity   = "public"
	defaultVersion     = gosnmp.Version2c
	defaultPort        = 161
	defaultRetries     = 1
	defaultTimeout     = defaultUpdateEvery
	defaultMaxOIDs     = 60
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("snmp", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *SNMP {
	return &SNMP{
		Config: Config{
			Hostname:  defaultHostname,
			Community: defaultCommunity,
			Options: Options{
				Port:    defaultPort,
				Retries: defaultRetries,
				Timeout: defaultUpdateEvery,
				Version: defaultVersion.String(),
				MaxOIDs: defaultMaxOIDs,
			},
		},
	}
}

type (
	Config struct {
		UpdateEvery int           `yaml:"update_every"`
		Hostname    string        `yaml:"hostname"`
		Community   string        `yaml:"community"`
		User        User          `yaml:"user"`
		Options     Options       `yaml:"options"`
		ChartsInput []ChartConfig `yaml:"charts"`
	}
	User struct {
		Name          string `yaml:"name"`
		SecurityLevel string `yaml:"level"`
		AuthProto     string `yaml:"auth_proto"`
		AuthKey       string `yaml:"auth_key"`
		PrivProto     string `yaml:"priv_proto"`
		PrivKey       string `yaml:"priv_key"`
	}
	Options struct {
		Port    int    `yaml:"port"`
		Retries int    `yaml:"retries"`
		Timeout int    `yaml:"timeout"`
		Version string `yaml:"version"`
		MaxOIDs int    `yaml:"max_request_size"`
	}
	ChartConfig struct {
		ID         string            `yaml:"id"`
		Title      string            `yaml:"title"`
		Units      string            `yaml:"units"`
		Family     string            `yaml:"family"`
		Type       string            `yaml:"type"`
		Priority   int               `yaml:"priority"`
		IndexRange []int             `yaml:"multiply_range"`
		Dimensions []DimensionConfig `yaml:"dimensions"`
	}
	DimensionConfig struct {
		OID        string `yaml:"oid"`
		Name       string `yaml:"name"`
		Algorithm  string `yaml:"algorithm"`
		Multiplier int    `yaml:"multiplier"`
		Divisor    int    `yaml:"divisor"`
	}
)

type SNMP struct {
	module.Base
	Config `yaml:",inline"`

	charts     *module.Charts
	snmpClient gosnmp.Handler
	oids       []string
}

func (s *SNMP) Init() bool {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation: %v", err)
		return false
	}

	snmpClient, err := s.initSNMPClient()
	if err != nil {
		s.Errorf("SNMP client initialization: %v", err)
		return false
	}

	s.Info(snmpClientConnInfo(snmpClient))

	err = snmpClient.Connect()
	if err != nil {
		s.Errorf("SNMP client connect: %v", err)
		return false
	}
	s.snmpClient = snmpClient

	charts, err := newCharts(s.ChartsInput)
	if err != nil {
		s.Errorf("Population of charts failed: %v", err)
		return false
	}
	s.charts = charts

	s.oids = s.initOIDs()

	return true
}

func (s *SNMP) Check() bool {
	return len(s.Collect()) > 0
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

func snmpClientConnInfo(c gosnmp.Handler) string {
	var info strings.Builder
	info.WriteString(fmt.Sprintf("hostname=%s,port=%d,snmp_version=%s", c.Target(), c.Port(), c.Version()))
	switch c.Version() {
	case gosnmp.Version1, gosnmp.Version2c:
		info.WriteString(fmt.Sprintf(",community=%s", c.Community()))
	case gosnmp.Version3:
		info.WriteString(fmt.Sprintf(",security_level=%d,%s", c.MsgFlags(), c.SecurityParameters().Description()))
	}
	return info.String()
}
