// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
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
			URL:     "ldap://127.0.0.1:389",
			Timeout: confopt.Duration(time.Second * 2),
		},

		newConn: newLdapConn,

		charts: charts.Copy(),
	}

}

type Config struct {
	UpdateEvery      int              `yaml:"update_every,omitempty" json:"update_every"`
	URL              string           `yaml:"url" json:"url"`
	Timeout          confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Username         string           `yaml:"username" json:"username"`
	Password         string           `yaml:"password" json:"password"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

type OpenLDAP struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	conn    ldapConn
	newConn func(Config) ldapConn
}

func (l *OpenLDAP) Configuration() any {
	return l.Config
}

func (l *OpenLDAP) Init() error {
	if l.URL == "" {
		return errors.New("empty LDAP server url")
	}
	if l.Username == "" {
		return errors.New("empty LDAP username")
	}

	return nil
}

func (l *OpenLDAP) Check() error {
	mx, err := l.collect()
	if err != nil {
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

func (l *OpenLDAP) Cleanup() {
	if l.conn != nil {
		if err := l.conn.disconnect(); err != nil {
			l.Warningf("error disconnecting ldap client: %v", err)
		}
		l.conn = nil
	}
}
