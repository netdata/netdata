// SPDX-License-Identifier: GPL-3.0-or-later

package dummy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

func NewDiscovery(cfg Config) (*Discovery, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %v", err)
	}
	d := &Discovery{
		Logger: logger.New().With(
			slog.String("component", "discovery"),
			slog.String("discoverer", "dummy"),
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

func (d *Discovery) groups() []*confgroup.Group {
	group := &confgroup.Group{Source: "internal"}

	for _, name := range d.names {
		def, ok := d.reg.Lookup(name)
		if !ok {
			continue
		}
		src := "internal"
		cfg := confgroup.Config{}
		cfg.SetModule(name)
		cfg.SetProvider("dummy")
		cfg.SetSourceType(confgroup.TypeStock)
		cfg.SetSource(src)
		cfg.ApplyDefaults(def)

		group.Configs = append(group.Configs, cfg)
	}

	return []*confgroup.Group{group}
}
