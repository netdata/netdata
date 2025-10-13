// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
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

// loadOverrides reads ALL *.yaml/*.yml files under getSnmpMetadataDir(),
// merges them in deterministic (alphabetical) order, and stores the result
// in overridesData. Later files win on conflicts.
func loadOverrides() {
	loadOverridesOnce.Do(func() {
		dir := getSnmpMetadataDir()
		if dir == "" {
			log.Warningf("snmp metadata overrides dir not found: %q (running without overrides)", dir)
			return
		}
		agg, err := loadOverridesFromDir(dir)
		if err != nil {
			log.Errorf("failed to load one or more metadata overrides from %s: %v", dir, err)
			return
		}

		overridesData = agg
	})
}

// loadOverridesFromDir scans dir recursively, parses all *.yaml|*.yml strictly,
// merges them into a single overrides.
func loadOverridesFromDir(dir string) (*overrides, error) {
	var agg overrides
	var loaded int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		bs, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var cur overrides
		if err := yaml.UnmarshalStrict(bs, &cur); err != nil {
			return fmt.Errorf("unmarshalling %s: %v", path, err)
		}

		loaded++
		mergeOverrides(&agg, &cur)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if loaded == 0 {
		return nil, errors.New("no snmp metadata overrides loaded")
	}

	return &agg, nil
}

func mergeOverrides(dst, src *overrides) {
	if src == nil {
		return
	}

	if len(src.EnterpriseNumbers.OrgToVendor) > 0 {
		if dst.EnterpriseNumbers.OrgToVendor == nil {
			dst.EnterpriseNumbers.OrgToVendor = make(map[string]string, len(src.EnterpriseNumbers.OrgToVendor))
		}
		for k, v := range src.EnterpriseNumbers.OrgToVendor {
			if v != "" {
				dst.EnterpriseNumbers.OrgToVendor[k] = v
			}
		}
	}

	if len(src.SysObjectIDs.CategoryMap) > 0 {
		if dst.SysObjectIDs.CategoryMap == nil {
			dst.SysObjectIDs.CategoryMap = make(map[string]string, len(src.SysObjectIDs.CategoryMap))
		}
		for k, v := range src.SysObjectIDs.CategoryMap {
			if v != "" {
				dst.SysObjectIDs.CategoryMap[k] = v
			}
		}
	}

	if len(src.SysObjectIDs.OIDOverrides) > 0 {
		if dst.SysObjectIDs.OIDOverrides == nil {
			dst.SysObjectIDs.OIDOverrides = make(map[string]sysObjectIDOverride, len(src.SysObjectIDs.OIDOverrides))
		}
		for oid, o := range src.SysObjectIDs.OIDOverrides {
			existing := dst.SysObjectIDs.OIDOverrides[oid]
			if o.Category != "" {
				existing.Category = o.Category
			}
			if o.Model != "" {
				existing.Model = o.Model
			}
			dst.SysObjectIDs.OIDOverrides[oid] = existing
		}
	}
}

func getSnmpMetadataDir() string {
	if executable.Name == "test" {
		return snmpMetadataDirFromThisFile()
	}
	if path := filepath.Join(executable.Directory, "..", "config", executable.Name, "snmp.profiles", "metadata"); isDirExists(path) {
		return path
	}
	return filepath.Join(pluginconfig.CollectorsStockDir(), "snmp.profiles", "metadata")
}

func snmpMetadataDirFromThisFile() string {
	// runtime.Caller(0) returns the absolute path to THIS .go file at build time.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	base := filepath.Dir(thisFile)

	// Try a couple of common repo layouts
	candidates := []string{
		filepath.Join(base, "..", "config", "go.d", "snmp.profiles", "metadata"),
		filepath.Join(base, "..", "..", "config", "go.d", "snmp.profiles", "metadata"),
	}

	for _, p := range candidates {
		if isDirExists(p) {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}

func isDirExists(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}
