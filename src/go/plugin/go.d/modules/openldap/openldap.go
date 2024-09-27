// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openldap", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *OpenLDAP {
	return &OpenLDAP{
		Config: Config{
			LDAP_URL: "ldap://localhost:389",
			Timeout:  confopt.Duration(time.Second * 2),
		},

		charts: OpenLdapCharts.Copy(),
	}

}

type Config struct {
	UpdateEvery       int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout           confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	DistinguishedName string           `yaml:"distinguished_name,omitempty" json:"distinguished_name,omitempty"`
	Password          string           `yaml:"password,omitempty" json:"password,omitempty"`
	LDAP_URL          string           `yaml:"ldap_url,omitempty" json:"ldap_url,omitempty"`
}

type OpenLDAP struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts
}

func (l *OpenLDAP) Configuration() any {
	return l.Config
}

func (l *OpenLDAP) Init() error {
	if l.LDAP_URL == "" {
		return errors.New("empty LDAP server url")
	}

	return nil
}

func (l *OpenLDAP) Check() error {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (l *OpenLDAP) Charts() *module.Charts {
	return l.charts
}

func (l *OpenLDAP) Collect() map[string]int64 {
	mx, err := l.collect()
	if err != nil {
		l.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (l *OpenLDAP) Cleanup() {}
