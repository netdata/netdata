// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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
		dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")
		return multipath.New(dir)
	}

	var userDir, stockDir string

	if userDir = handleDirOnWin(os.Getenv("NETDATA_USER_CONFIG_DIR")); userDir != "" {
		if dir := filepath.Join(userDir, "go.d/snmp.profiles"); isDirExists(dir) {
			userDir = dir
		}
	}
	if stockDir = handleDirOnWin(os.Getenv("NETDATA_STOCK_CONFIG_DIR")); stockDir != "" {
		if dir := filepath.Join(stockDir, "go.d/snmp.profiles/default"); isDirExists(dir) {
			stockDir = dir
		}
	}

	if userDir != "" || stockDir != "" {
		return multipath.New(userDir, stockDir)
	}

	// Development: When running from source (netdata/src/go/plugin/go.d/bin)
	// Looks for profiles in the local git repository
	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.profiles/default"); isDirExists(dir) {
		return multipath.New(dir)
	}

	possibleDirs := []string{
		filepath.Join(executable.Directory, "../../../../etc/netdata/go.d/snmp.profiles"),
		// User Standard installation paths
		handleDirOnWin("/etc/netdata/go.d/snmp.profiles"),
		handleDirOnWin("/opt/netdata/etc/netdata/go.d/snmp.profiles"),

		filepath.Join(executable.Directory, "../../../lib/netdata/conf.d/go.d/snmp.profiles/default"),
		// Stock standard installation paths
		handleDirOnWin("/usr/lib/netdata/conf.d/go.d/snmp.profiles/default"),
		handleDirOnWin("/opt/netdata/usr/lib/netdata/conf.d/go.d/snmp.profiles/default"),
	}

	for _, dir := range possibleDirs {
		isStock := strings.HasSuffix(filepath.Base(dir), "default")
		switch {
		case userDir == "" && !isStock && isDirExists(dir):
			userDir = dir
		case stockDir == "" && isStock && isDirExists(dir):
			stockDir = dir
		}
	}

	return multipath.New(userDir, stockDir)
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}

func handleDirOnWin(path string) string {
	base := os.Getenv("NETDATA_CYGWIN_BASE_PATH")

	// TODO: temp workaround for debug mode
	if base == "" && strings.HasPrefix(executable.Directory, "C:\\msys64") {
		base = "C:\\msys64"
	}

	if base == "" || !strings.HasPrefix(path, "/") {
		return path
	}

	return filepath.Join(base, path)
}
