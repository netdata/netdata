// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

var log = logger.New().With("component", "snmp/ddsnmp")

var (
	ddProfiles []*Profile
	loadOnce   sync.Once
)

func load() {
	loadOnce.Do(func() {
		dir := getProfilesDir()

		profiles, err := loadProfiles(dir)
		if err != nil {
			log.Errorf("failed to loadProfiles dd snmp profiles: %v", err)
			return
		}

		if len(profiles) == 0 {
			log.Warningf("no dd snmp profiles found in '%s'", dir)
			return
		}

		log.Infof("found %d profiles in '%s'", len(profiles), dir)
		ddProfiles = profiles
	})
}

func loadProfiles(dirpath string) ([]*Profile, error) {
	var profiles []*Profile

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !(strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml")) {
			return nil
		}

		profile, err := loadProfile(path)
		if err != nil {
			log.Warningf("invalid profile '%s': %v", path, err)
			return nil
		}

		if err := profile.validate(); err != nil {
			log.Warningf("invalid profile '%s': %v", path, err)
			return nil
		}

		profiles = append(profiles, profile)
		return nil
	}); err != nil {
		return nil, err
	}

	return profiles, nil
}

func loadProfile(filename string) (*Profile, error) {
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

	dir := filepath.Dir(filename)

	processedExtends := make(map[string]bool)
	if err := loadProfileExtensions(&prof, dir, processedExtends); err != nil {
		return nil, err
	}

	return &prof, nil
}

func loadProfileExtensions(profile *Profile, dir string, processedExtends map[string]bool) error {
	for _, name := range profile.Definition.Extends {
		if processedExtends[name] {
			continue
		}
		processedExtends[name] = true

		baseProf, err := loadProfile(filepath.Join(dir, name))
		if err != nil {
			return err
		}

		if err := loadProfileExtensions(baseProf, dir, processedExtends); err != nil {
			return err
		}

		profile.merge(baseProf)
	}

	return nil
}

func getProfilesDir() string {
	if executable.Name == "test" {
		dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")
		return dir
	}
	if dir := os.Getenv("NETDATA_STOCK_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "go.d/snmp.profiles/default")
	}
	return filepath.Join(executable.Directory, "../../../config/go.d/snmp.profiles/default")
}
