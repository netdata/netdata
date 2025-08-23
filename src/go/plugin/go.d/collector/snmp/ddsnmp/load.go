// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pluginconfig"
)

var log = logger.New().With("component", "snmp/ddsnmp")

var (
	// Profile loading is intentionally global and cached to avoid reloading
	// profiles for each SNMP job instance. This is a performance optimization
	// as there can be many concurrent SNMP collection jobs.
	ddProfiles []*Profile
	loadOnce   sync.Once
)

func loadProfiles() {
	loadOnce.Do(func() {
		profilesPaths := getProfilesDirs()
		log.Infof("Loading SNMP profiles from %v", profilesPaths)
		seen := make(map[string]bool)

		for _, dir := range profilesPaths {
			profiles, err := loadProfilesFromDir(dir, profilesPaths)
			if err != nil {
				log.Errorf("failed to load dd snmp profiles from '%s': %v", dir, err)
				continue
			}

			if len(profiles) == 0 {
				log.Infof("no dd snmp profiles found in '%s'", dir)
				continue
			}

			log.Infof("found %d profiles in '%s'", len(profiles), dir)
			profiles = slices.DeleteFunc(profiles, func(p *Profile) bool {
				name := filepath.Base(p.SourceFile)
				if seen[name] {
					log.Infof("duplicate profile '%s' found in '%s', not adding it", name, dir)
					return true
				}
				seen[name] = true
				return false
			})
			ddProfiles = append(ddProfiles, profiles...)
		}

		if len(ddProfiles) == 0 {
			log.Warningf("no dd snmp profiles found in any of the searched directories: %v", profilesPaths)
		} else {
			log.Infof("loaded %d dd snmp profiles total", len(ddProfiles))
		}
	})
}

func loadProfilesFromDir(dirpath string, extendsPaths multipath.MultiPath) ([]*Profile, error) {
	var profiles []*Profile

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !(strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml")) {
			return nil
		}
		// Skip abstract profiles
		if strings.HasPrefix(d.Name(), "_") {
			return nil
		}

		profile, err := loadProfile(path, extendsPaths)
		if err != nil {
			log.Warningf("invalid profile '%s': %v", path, err)
			return nil
		}

		if err := profile.validate(); err != nil {
			log.Warningf("invalid profile '%s': %v", path, err)
			return nil
		}
		if err := CompileTransforms(profile); err != nil {
			log.Warningf("invalid profile '%s': %v", path, err)
			return nil
		}

		profile.removeConstantMetrics()

		profiles = append(profiles, profile)
		return nil
	}); err != nil {
		return nil, err
	}

	return profiles, nil
}

func loadProfile(filename string, extendsPaths multipath.MultiPath) (*Profile, error) {
	return loadProfileWithExtendsMap(filename, extendsPaths, []string{})
}

func loadProfileWithExtendsMap(filename string, extendsPaths multipath.MultiPath, stack []string) (*Profile, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var prof Profile
	if err := yaml.Unmarshal(content, &prof.Definition); err != nil {
		return nil, err
	}

	if prof.SourceFile == "" {
		prof.SourceFile, _ = filepath.Abs(filename)
	}

	// Handle empty profiles - these are profiles where content has been deliberately removed,
	// but the file itself is preserved. This ensures that when users update, their existing
	// profile files are overwritten with empty content rather than being left with stale data.
	if prof.Definition == nil {
		prof.Definition = &ddprofiledefinition.ProfileDefinition{}
		return &prof, nil
	}

	prof.extensionHierarchy = make([]*extensionInfo, 0, len(prof.Definition.Extends))

	for _, name := range prof.Definition.Extends {
		if slices.Contains(stack, name) {
			return nil, fmt.Errorf("circular extends detected: '%s' already included (in file: %s)", name, prof.SourceFile)
		}

		extPath, err := extendsPaths.Find(name)
		if err != nil {
			return nil, fmt.Errorf("cannot find extension '%s': %w", name, err)
		}

		mergedBase, err := loadProfileWithExtendsMap(extPath, extendsPaths, append(stack, name))
		if err != nil {
			return nil, err
		}

		extInfo := &extensionInfo{
			name:       name,
			sourceFile: mergedBase.SourceFile,
			extensions: mergedBase.extensionHierarchy,
		}
		prof.extensionHierarchy = append(prof.extensionHierarchy, extInfo)

		prof.merge(mergedBase)
	}

	return &prof, nil
}

func getProfilesDirs() multipath.MultiPath {
	if executable.Name == "test" {
		return multipath.New(snmpProfilesDirFromThisFile())
	}

	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.profiles/default"); isDirExists(dir) {
		return multipath.New(dir)
	}

	var dirs []string
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		dirs = append(dirs, filepath.Join(dir, "snmp.profiles"))
	}
	dirs = append(dirs, filepath.Join(pluginconfig.CollectorsStockDir(), "snmp.profiles", "default"))

	return multipath.New(dirs...)
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}

func snmpProfilesDirFromThisFile() string {
	// runtime.Caller(0) returns the absolute path to THIS .go file at build time.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	base := filepath.Dir(thisFile)

	candidates := []string{
		filepath.Join(base, "..", "..", "..", "config", "go.d", "snmp.profiles", "default"),
	}

	for _, p := range candidates {
		if isDirExists(p) {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}
