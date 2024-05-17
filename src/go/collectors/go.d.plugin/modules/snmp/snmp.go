// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"

	"github.com/gosnmp/gosnmp"
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
		Config: func() any { return &Config{} },
	})
}

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
			User: User{
				Name:          "",
				SecurityLevel: "authPriv",
				AuthProto:     "sha512",
				AuthKey:       "",
				PrivProto:     "aes192c",
				PrivKey:       "",
			},
		},
	}
}

type (
	Config struct {
		UpdateEvery int           `yaml:"update_every,omitempty" json:"update_every"`
		Hostname    string        `yaml:"hostname" json:"hostname"`
		Community   string        `yaml:"community,omitempty" json:"community"`
		User        User          `yaml:"user,omitempty" json:"user"`
		Options     Options       `yaml:"options,omitempty" json:"options"`
		ChartsInput []ChartConfig `yaml:"charts,omitempty" json:"charts"`
	}
	User struct {
		Name          string `yaml:"name,omitempty" json:"name"`
		SecurityLevel string `yaml:"level,omitempty" json:"level"`
		AuthProto     string `yaml:"auth_proto,omitempty" json:"auth_proto"`
		AuthKey       string `yaml:"auth_key,omitempty" json:"auth_key"`
		PrivProto     string `yaml:"priv_proto,omitempty" json:"priv_proto"`
		PrivKey       string `yaml:"priv_key,omitempty" json:"priv_key"`
	}
	Options struct {
		Port    int    `yaml:"port,omitempty" json:"port"`
		Retries int    `yaml:"retries,omitempty" json:"retries"`
		Timeout int    `yaml:"timeout,omitempty" json:"timeout"`
		Version string `yaml:"version,omitempty" json:"version"`
		MaxOIDs int    `yaml:"max_request_size,omitempty" json:"max_request_size"`
	}
	ChartConfig struct {
		ID         string            `yaml:"id" json:"id"`
		Title      string            `yaml:"title" json:"title"`
		Units      string            `yaml:"units" json:"units"`
		Family     string            `yaml:"family" json:"family"`
		Type       string            `yaml:"type" json:"type"`
		Priority   int               `yaml:"priority" json:"priority"`
		IndexRange []int             `yaml:"multiply_range,omitempty" json:"multiply_range"`
		Dimensions []DimensionConfig `yaml:"dimensions" json:"dimensions"`
	}
	DimensionConfig struct {
		OID        string `yaml:"oid" json:"oid"`
		Name       string `yaml:"name" json:"name"`
		Algorithm  string `yaml:"algorithm" json:"algorithm"`
		Multiplier int    `yaml:"multiplier" json:"multiplier"`
		Divisor    int    `yaml:"divisor" json:"divisor"`
	}
)

type SNMP struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	snmpClient gosnmp.Handler

	oids []string
}

func (s *SNMP) Configuration() any {
	return s.Config
}

func (s *SNMP) Init() error {
	err := s.validateConfig()
	if err != nil {
		s.Errorf("config validation: %v", err)
		return err
	}

	snmpClient, err := s.initSNMPClient()
	if err != nil {
		s.Errorf("SNMP client initialization: %v", err)
		return err
	}

	s.Info(snmpClientConnInfo(snmpClient))

	err = snmpClient.Connect()
	if err != nil {
		s.Errorf("SNMP client connect: %v", err)
		return err
	}
	s.snmpClient = snmpClient

	charts, err := newCharts(s.ChartsInput)
	if err != nil {
		s.Errorf("Population of charts failed: %v", err)
		return err
	}
	s.charts = charts

	s.oids = s.initOIDs()

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
