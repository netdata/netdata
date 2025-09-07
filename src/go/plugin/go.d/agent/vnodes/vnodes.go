// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/logger"
)

//go:embed "config_schema.json"
var ConfigSchema string

var log = logger.New().With(
	slog.String("component", "vnodes"),
)

func Load(dir string) map[string]*VirtualNode {
	return readConfDir(dir)
}

type VirtualNode struct {
	Name     string            `yaml:"name" json:"name"`
	Hostname string            `yaml:"hostname" json:"hostname"`
	GUID     string            `yaml:"guid" json:"guid"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels"`

	Source     string `yaml:"-" json:"-"`
	SourceType string `yaml:"-" json:"-"`
}

func (v *VirtualNode) Copy() *VirtualNode {
	if v == nil {
		return nil
	}

	labels := make(map[string]string, len(v.Labels))
	maps.Copy(labels, v.Labels)

	return &VirtualNode{
		Name:       v.Name,
		Hostname:   v.Hostname,
		GUID:       v.GUID,
		Source:     v.Source,
		SourceType: v.SourceType,
		Labels:     labels,
	}
}

func (v *VirtualNode) Equal(vn *VirtualNode) bool {
	return v.Name == vn.Name &&
		v.Hostname == vn.Hostname &&
		v.GUID == vn.GUID &&
		maps.Equal(v.Labels, vn.Labels)
}

func readConfDir(dir string) map[string]*VirtualNode {
	vnodes := make(map[string]*VirtualNode)

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Warning(err)
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			dst, err := os.Readlink(path)
			if err != nil {
				log.Warningf("failed to resolve symlink '%s': %v", path, err)
				return nil
			}

			if !filepath.IsAbs(dst) {
				dst = filepath.Join(filepath.Dir(path), filepath.Clean(dst))
			}

			fi, err := os.Stat(dst)
			if err != nil {
				log.Warningf("failed to stat resolved path '%s': %v", dst, err)
				return nil
			}
			if !fi.Mode().IsRegular() {
				log.Debugf("'%s' is not a regular file, skipping it", dst)
				return nil
			}
			path = dst
		} else if !d.Type().IsRegular() {
			log.Debugf("'%s' is not a regular file, skipping it", path)
			return nil
		}

		if !isConfigFile(path) {
			log.Debugf("'%s' is not a config file (wrong extension), skipping it", path)
			return nil
		}

		var cfg []VirtualNode

		if err := loadConfigFile(&cfg, path); err != nil {
			log.Warning(err)
			return nil
		}

		for _, v := range cfg {
			if v.Hostname == "" || v.GUID == "" {
				log.Warningf("skipping virtual node '%+v': required fields are missing (%s)", v, path)
				continue
			}
			if err := uuid.Validate(v.GUID); err != nil {
				log.Warningf("skipping virtual node '%+v': invalid GUID: %v (%s)", v, err, path)
				continue
			}
			if _, ok := vnodes[v.Hostname]; ok {
				log.Warningf("skipping virtual node '%+v': duplicate node (%s)", v, path)
				continue
			}

			v := v

			if v.Name == "" {
				v.Name = v.Hostname
			}
			v.Source = fmt.Sprintf("file=%s", path)
			if isStockConfig(path) {
				v.SourceType = "stock"
			} else {
				v.SourceType = "user"
			}

			log.Debugf("adding virtual node'%+v' (%s)", v, path)
			vnodes[v.Hostname] = &v
		}

		return nil
	})

	return vnodes
}

func isConfigFile(path string) bool {
	switch filepath.Ext(path) {
	case ".yaml", ".yml", ".conf":
		return true
	default:
		return false
	}
}

func loadConfigFile(conf any, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if err := yaml.NewDecoder(f).Decode(conf); err != nil && err != io.EOF {
		return err
	}

	return nil
}

var (
	envNDStockConfigDir = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
)

func isStockConfig(path string) bool {
	if envNDStockConfigDir == "" {
		return false
	}
	return strings.HasPrefix(path, envNDStockConfigDir)
}
