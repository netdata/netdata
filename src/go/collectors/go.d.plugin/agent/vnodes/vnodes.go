// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/netdata/go.d.plugin/logger"

	"gopkg.in/yaml.v2"
)

var Disabled = false // TODO: remove after Netdata v1.39.0. Fix for "from source" stable-channel installations.

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
		mux     *sync.Mutex
		vnodes  map[string]*VirtualNode
	}
	VirtualNode struct {
		GUID     string            `yaml:"guid"`
		Hostname string            `yaml:"hostname"`
		Labels   map[string]string `yaml:"labels"`
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

		if !d.Type().IsRegular() || !isConfigFile(path) {
			return nil
		}

		var cfg []VirtualNode
		if err := loadConfigFile(&cfg, path); err != nil {
			vn.Warning(err)
			return nil
		}

		for _, v := range cfg {
			if v.Hostname == "" || v.GUID == "" {
				vn.Warningf("skipping virtual node '%+v': some required fields are missing (%s)", v, path)
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

func loadConfigFile(conf interface{}, path string) error {
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
