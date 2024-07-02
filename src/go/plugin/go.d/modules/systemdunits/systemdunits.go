// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/coreos/go-systemd/v22/dbus"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("systemdunits", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10, // gathering systemd units can be a CPU intensive op
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *SystemdUnits {
	return &SystemdUnits{
		Config: Config{
			Timeout:               web.Duration(time.Second * 2),
			Include:               []string{"*.service"},
			SkipTransient:         false,
			CollectUnitFiles:      false,
			IncludeUnitFiles:      []string{"*.service"},
			CollectUnitFilesEvery: web.Duration(time.Minute * 5),
		},
		charts:        &module.Charts{},
		client:        newSystemdDBusClient(),
		seenUnits:     make(map[string]bool),
		unitTransient: make(map[string]bool),
		seenUnitFiles: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery           int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout               web.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Include               []string     `yaml:"include,omitempty" json:"include"`
	SkipTransient         bool         `yaml:"skip_transient" json:"skip_transient"`
	CollectUnitFiles      bool         `yaml:"collect_unit_files" json:"collect_unit_files"`
	IncludeUnitFiles      []string     `yaml:"include_unit_files,omitempty" json:"include_unit_files"`
	CollectUnitFilesEvery web.Duration `yaml:"collect_unit_files_every,omitempty" json:"collect_unit_files_every"`
}

type SystemdUnits struct {
	module.Base
	Config `yaml:",inline" json:""`

	client systemdClient
	conn   systemdConnection

	systemdVersion int

	seenUnits     map[string]bool
	unitTransient map[string]bool
	unitSr        matcher.Matcher

	lastListUnitFilesTime time.Time
	cachedUnitFiles       []dbus.UnitFile
	seenUnitFiles         map[string]bool

	charts *module.Charts
}

func (s *SystemdUnits) Configuration() any {
	return s.Config
}

func (s *SystemdUnits) Init() error {
	if err := s.validateConfig(); err != nil {
		s.Errorf("config validation: %v", err)
		return err
	}

	sr, err := s.initUnitSelector()
	if err != nil {
		s.Errorf("init unit selector: %v", err)
		return err
	}
	s.unitSr = sr

	s.Debugf("timeout: %s", s.Timeout)
	s.Debugf("units: patterns '%v'", s.Include)
	s.Debugf("unit files: enabled '%v', every '%s', patterns: %v",
		s.CollectUnitFiles, s.CollectUnitFilesEvery, s.IncludeUnitFiles)

	return nil
}

func (s *SystemdUnits) Check() error {
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

func (s *SystemdUnits) Charts() *module.Charts {
	return s.charts
}

func (s *SystemdUnits) Collect() map[string]int64 {
	mx, err := s.collect()
	if err != nil {
		s.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (s *SystemdUnits) Cleanup() {
	s.closeConnection()
}
