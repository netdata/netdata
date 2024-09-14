// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/logger"

	"github.com/google/uuid"
	"gopkg.in/yaml.v2"
)

func New(confDir string) *Vnodes {
	vn := &Vnodes{
		Logger: logger.New().With(
			slog.String("component", "vnodes"),
		),

		confDir: confDir,
		vnodes:  make(map[string]*VirtualNode),
	}

	vn.readConfDir()

	return vn
}

type (
	Vnodes struct {
		*logger.Logger

		confDir string
		vnodes  map[string]*VirtualNode
	}
	VirtualNode struct {
		GUID     string            `yaml:"guid" json:"guid"`
		Hostname string            `yaml:"hostname" json:"hostname"`
		Labels   map[string]string `yaml:"labels" json:"labels"`
	}
)

func (vn *Vnodes) Lookup(key string) (*VirtualNode, bool) {
	v, ok := vn.vnodes[key]
	return v, ok
}

func (vn *Vnodes) Len() int {
	return len(vn.vnodes)
}

func (vn *Vnodes) readConfDir() {
	_ = filepath.WalkDir(vn.confDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			vn.Warning(err)
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			dst, err := os.Readlink(path)
			if err != nil {
				vn.Warningf("failed to resolve symlink '%s': %v", path, err)
				return nil
			}

			if !filepath.IsAbs(dst) {
				dst = filepath.Join(filepath.Dir(path), filepath.Clean(dst))
			}

			fi, err := os.Stat(dst)
			if err != nil {
				vn.Warningf("failed to stat resolved path '%s': %v", dst, err)
				return nil
			}
			if !fi.Mode().IsRegular() {
				vn.Debugf("'%s' is not a regular file, skipping it", dst)
				return nil
			}
			path = dst
		} else if !d.Type().IsRegular() {
			vn.Debugf("'%s' is not a regular file, skipping it", path)
			return nil
		}

		if !isConfigFile(path) {
			vn.Debugf("'%s' is not a config file (wrong extension), skipping it", path)
			return nil
		}

		var cfg []VirtualNode

		if err := loadConfigFile(&cfg, path); err != nil {
			vn.Warning(err)
			return nil
		}

		for _, v := range cfg {
			if v.Hostname == "" || v.GUID == "" {
				vn.Warningf("skipping virtual node '%+v': required fields are missing (%s)", v, path)
				continue
			}
			if err := uuid.Validate(v.GUID); err != nil {
				vn.Warningf("skipping virtual node '%+v': invalid GUID: %v (%s)", v, err, path)
				continue
			}
			if _, ok := vn.vnodes[v.Hostname]; ok {
				vn.Warningf("skipping virtual node '%+v': duplicate node (%s)", v, path)
				continue
			}

			v := v
			vn.Debugf("adding virtual node'%+v' (%s)", v, path)
			vn.vnodes[v.Hostname] = &v
		}

		return nil
	})
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
