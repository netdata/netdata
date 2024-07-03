// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
)

type (
	staticConfig struct {
		confgroup.Default `yaml:",inline"`
		Jobs              []confgroup.Config `yaml:"jobs"`
	}
	sdConfig []confgroup.Config
)

func NewReader(reg confgroup.Registry, paths []string) *Reader {
	return &Reader{
		Logger: log,
		reg:    reg,
		paths:  paths,
	}
}

type Reader struct {
	*logger.Logger

	reg   confgroup.Registry
	paths []string
}

func (r *Reader) String() string {
	return r.Name()
}

func (r *Reader) Name() string {
	return "file reader"
}

func (r *Reader) Run(ctx context.Context, in chan<- []*confgroup.Group) {
	r.Info("instance is started")
	defer func() { r.Info("instance is stopped") }()

	select {
	case <-ctx.Done():
	case in <- r.groups():
	}

	close(in)
}

func (r *Reader) groups() (groups []*confgroup.Group) {
	for _, pattern := range r.paths {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, path := range matches {
			if fi, err := os.Stat(path); err != nil || !fi.Mode().IsRegular() {
				continue
			}

			group, err := parse(r.reg, path)
			if err != nil {
				r.Warningf("parse '%s': %v", path, err)
				continue
			}

			if group == nil {
				group = &confgroup.Group{Source: path}
			} else {
				for _, cfg := range group.Configs {
					cfg.SetProvider("file reader")
					cfg.SetSourceType(configSourceType(path))
					cfg.SetSource(fmt.Sprintf("discoverer=file_reader,file=%s", path))
				}
			}
			groups = append(groups, group)
		}
	}

	return groups
}

func configSourceType(path string) string {
	if strings.Contains(path, "/etc/netdata") {
		return "user"
	}
	return "stock"
}
