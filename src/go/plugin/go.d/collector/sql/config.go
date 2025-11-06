// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type Config struct {
	Driver          string           `yaml:"driver" json:"driver"`
	DSN             string           `json:"dsn" yaml:"dsn"`
	Timeout         confopt.Duration `yaml:"timeout" json:"timeout"`
	MaxOpenConns    int              `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int              `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime confopt.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`

	Queries []QueryConfig `yaml:"queries" json:"queries"`
}

type QueryConfig struct {
	Name      string          `yaml:"name" json:"name"`
	Labels    []string        `yaml:"labels" json:"labels"`
	Values    []string        `yaml:"values" json:"values"`
	ChartMeta ChartMetaConfig `yaml:"chart_meta" json:"chart_meta"`
	Query     string          `yaml:"query" json:"query"`
}

type ChartMetaConfig struct {
	Context     string `yaml:"context" json:"context"`
	Description string `yaml:"description" json:"description"`
	Family      string `yaml:"family" json:"family"`
	Units       string `yaml:"units" json:"units"`
	Type        string `yaml:"type" json:"type"`
	Algorithm   string `yaml:"algorithm" json:"algorithm"`
}
