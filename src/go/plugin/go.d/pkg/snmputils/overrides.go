// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pluginconfig"
)

var (
	overridesData     *overrides
	loadOverridesOnce sync.Once
)

type (
	overrides struct {
		EnterpriseNumbers enterpriseNumbersOverrides `yaml:"enterprisenumbers"`
		SysObjectIDs      sysObjectIDOverrides       `yaml:"sysobjectids"`
	}
	enterpriseNumbersOverrides struct {
		// Map PEN org names -> canonical vendor names
		OrgToVendor map[string]string `yaml:"org_to_vendor"`
	}
	sysObjectIDOverrides struct {
		// Exact OID overrides
		OIDOverrides map[string]sysObjectIDOverride `yaml:"oid_overrides"`
		// Category normalization
		CategoryMap map[string]string `yaml:"category_map"`
	}
	sysObjectIDOverride struct {
		// If non-empty, overrides the value; empty string => no change
		Category string `yaml:"category,omitempty"`
		Model    string `yaml:"model,omitempty"`
	}
)

func loadOverrides() {
	loadOverridesOnce.Do(func() {
		bs, err := os.ReadFile(getOverridesPath())
		if err != nil {
			log.Errorf("failed to read snmp overrides file: %v", err)
			return
		}
		var o overrides
		if err := yaml.UnmarshalStrict(bs, &o); err != nil {
			log.Errorf("failed to unmarshal snmp overrides file: %v", err)
			return
		}
		overridesData = &o
	})
}

func getOverridesPath() string {
	if executable.Name == "test" {
		dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/overrides.yaml")
		return dir
	}
	if path := filepath.Join(executable.Directory, "..", "config", executable.Name, "snmp.profiles", "overrides.yaml"); isFileExists(path) {
		return path
	}
	return filepath.Join(pluginconfig.CollectorsStockDir(), "snmp.profiles", "overrides.yaml")
}

func isFileExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsRegular()
}
