// SPDX-License-Identifier: GPL-3.0-or-later

package dummy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/netdata/go.d.plugin/agent/confgroup"
	"github.com/netdata/go.d.plugin/logger"
)

func NewDiscovery(cfg Config) (*Discovery, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %v", err)
	}
	d := &Discovery{
		Logger: logger.New().With(
			slog.String("component", "discovery dummy"),
		),
		reg:   cfg.Registry,
		names: cfg.Names,
	}
	return d, nil
}

type Discovery struct {
	*logger.Logger

	reg   confgroup.Registry
	names []string
}

func (d *Discovery) String() string {
	return d.Name()
}

func (d *Discovery) Name() string {
	return "dummy discovery"
}

func (d *Discovery) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	d.Info("instance is started")
	defer func() { d.Info("instance is stopped") }()

	select {
	case <-ctx.Done():
	case in <- d.groups():
	}

	close(in)
}

func (d *Discovery) groups() (groups []*confgroup.Group) {
	for _, name := range d.names {
		groups = append(groups, d.newCfgGroup(name))
	}
	return groups
}

func (d *Discovery) newCfgGroup(name string) *confgroup.Group {
	def, ok := d.reg.Lookup(name)
	if !ok {
		return nil
	}

	cfg := confgroup.Config{}
	cfg.SetModule(name)
	cfg.SetSource(name)
	cfg.SetProvider("dummy")
	cfg.Apply(def)

	group := &confgroup.Group{
		Configs: []confgroup.Config{cfg},
		Source:  name,
	}
	return group
}
