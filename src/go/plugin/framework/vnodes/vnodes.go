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
	"strconv"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
)

//go:embed "config_schema.json"
var ConfigSchema string

var log = logger.New().With(
	slog.String("component", "vnodes"),
)

func Load(dir string, snmpSupported bool) map[string]*Config {
	return readConfDir(dir, snmpSupported)
}

type VirtualNode struct {
	Name       string            `yaml:"name"                  json:"name"`
	Hostname   string            `yaml:"hostname"              json:"hostname"`
	GUID       string            `yaml:"guid"                  json:"guid"`
	Labels     map[string]string `yaml:"labels,omitempty"      json:"labels"`
	StaleAfter *confopt.Duration `yaml:"stale_after,omitempty" json:"stale_after,omitempty"`

	Source     string `yaml:"-" json:"-"`
	SourceType string `yaml:"-" json:"-"`
}

func (v *VirtualNode) Copy() *VirtualNode {
	if v == nil {
		return nil
	}

	labels := make(map[string]string, len(v.Labels))
	maps.Copy(labels, v.Labels)
	var staleAfter *confopt.Duration
	if v.StaleAfter != nil {
		value := *v.StaleAfter
		staleAfter = &value
	}

	return &VirtualNode{
		Name:       v.Name,
		Hostname:   v.Hostname,
		GUID:       v.GUID,
		Source:     v.Source,
		SourceType: v.SourceType,
		Labels:     labels,
		StaleAfter: staleAfter,
	}
}

func (v *VirtualNode) Equal(vn *VirtualNode) bool {
	return v.Name == vn.Name &&
		v.Hostname == vn.Hostname &&
		v.GUID == vn.GUID &&
		staleAfterEqual(v.StaleAfter, vn.StaleAfter) &&
		maps.Equal(v.Labels, vn.Labels)
}

func staleAfterEqual(a, b *confopt.Duration) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// HostLabels materializes lifecycle configuration without changing the operator's labels.
func (v *VirtualNode) HostLabels() map[string]string {
	labels := maps.Clone(v.Labels)
	if v.StaleAfter == nil {
		return labels
	}
	if *v.StaleAfter == 0 {
		delete(labels, "_node_stale_after_seconds")
		return labels
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["_node_stale_after_seconds"] = strconv.FormatInt(int64(v.StaleAfter.Duration()/time.Second), 10)
	return labels
}

func readConfDir(dir string, snmpSupported bool) map[string]*Config {
	vnodes := make(map[string]*Config)
	guids := make(map[string]string)
	hostnames := make(map[string]string)

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

		var cfg []Config

		if err := loadConfigFile(&cfg, path); err != nil {
			log.Warningf("invalid vnode configuration file %q", path)
			return nil
		}

		for _, v := range cfg {
			if v.IsSNMP() && !snmpSupported {
				log.Debugf("skipping virtual node %q: SNMP acquisition is unavailable in this plugin", v.Name)
				continue
			}

			if !v.IsSNMP() && v.Name != "" && v.Name != v.Hostname {
				log.Warningf(
					"ignoring virtual node name '%s' for hostname '%s'; file-based vnode identity uses hostname",
					v.Name, v.Hostname,
				)
			}
			if !v.IsSNMP() {
				v.Name = v.Hostname
			}
			v.Source = fmt.Sprintf("file=%s", path)
			if isStockConfig(path) {
				v.SourceType = "stock"
			} else {
				v.SourceType = "user"
			}
			v.NormalizeCredentials()
			err := v.Validate()
			guidKey := v.IdentityGUID()
			if err == nil {
				guidKey, err = ConfiguredGUIDKey(guidKey)
			}
			if err != nil {
				log.Warningf("skipping virtual node %q: %v (%s)", v.Name, err, path)
				continue
			}
			if _, ok := vnodes[v.Name]; ok {
				log.Warningf("skipping virtual node %q: duplicate name (%s)", v.Name, path)
				continue
			}
			if other, ok := guids[guidKey]; ok {
				log.Warningf(
					"skipping virtual node %q: duplicate GUID already used by %q (%s)",
					v.Name, other, path,
				)
				continue
			}

			if v.Hostname != "" {
				if _, exists := hostnames[v.Hostname]; exists {
					log.Warningf("skipping virtual node %q: duplicate hostname (%s)", v.Name, path)
					continue
				}
				hostnames[v.Hostname] = v.Name
			}
			log.Debugf("adding virtual node %q (%s)", v.Name, path)
			vnodes[v.Name] = &v
			guids[guidKey] = v.Name
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

func isStockConfig(path string) bool {
	return pluginconfig.IsStock(path)
}
