// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

const (
	defaultJitterEWMASamples = 16
	defaultJitterSMAWindow   = 10
)

type ProbeConfig struct {
	Network    string           `yaml:"network,omitempty" json:"network"`
	Interface  string           `yaml:"interface,omitempty" json:"interface"`
	Privileged bool             `yaml:"privileged" json:"privileged"`
	Packets    int              `yaml:"packets,omitempty" json:"packets"`
	Interval   confopt.Duration `yaml:"interval,omitempty" json:"interval"`
	Timeout    time.Duration    `yaml:"-,omitempty" json:",omitempty"`
}

type AnalysisConfig struct {
	JitterEWMASamples int `yaml:"jitter_ewma_samples,omitempty" json:"jitter_ewma_samples"`
	JitterSMAWindow   int `yaml:"jitter_sma_window,omitempty" json:"jitter_sma_window"`
}

type Config struct {
	Probe    ProbeConfig
	Analysis AnalysisConfig
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.Probe.Packets <= 0 {
		return Config{}, errors.New("probe packets must be > 0")
	}
	if cfg.Probe.Interval.Duration() <= 0 {
		return Config{}, errors.New("probe interval must be > 0")
	}
	if cfg.Probe.Timeout <= 0 {
		return Config{}, errors.New("probe timeout must be > 0")
	}

	if cfg.Analysis.JitterEWMASamples <= 0 {
		cfg.Analysis.JitterEWMASamples = defaultJitterEWMASamples
	}
	if cfg.Analysis.JitterSMAWindow <= 0 {
		cfg.Analysis.JitterSMAWindow = defaultJitterSMAWindow
	}

	return cfg, nil
}
