// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("samba", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Samba {
	return &Samba{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts: &module.Charts{},
		once:   &sync.Once{},
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type Samba struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts
	once   *sync.Once

	exec smbStatusBinary
}

func (s *Samba) Configuration() any {
	return s.Config
}

func (s *Samba) Init() error {
	smbStatus, err := s.initSmbStatusBinary()
	if err != nil {
		return fmt.Errorf("smbstatus exec initialization: %v", err)
	}
	s.exec = smbStatus

	return nil
}

func (s *Samba) Check() error {
	mx, err := s.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (s *Samba) Charts() *module.Charts {
	return s.charts
}

func (s *Samba) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (s *Samba) Cleanup() {}
