// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

var log = logger.New().With("component", "snmp/ddsnmp")

var ddProfiles []*Profile

func init() {
	dir := os.Getenv("NETDATA_STOCK_CONFIG_DIR")
	if dir != "" {
		dir = filepath.Join(dir, "go.d/snmp.profiles/default")
	} else {
		if dir, _ = filepath.Abs("../../../config/go.d/snmp.profiles/default"); !isDirExists(dir) {
			dir = filepath.Join(executable.Directory, "../../../../usr/lib/netdata/conf.d/go.d/snmp.profiles/default")
		}
	}
	profiles, err := load(dir)
	if err != nil {
		log.Errorf("failed to load dd snmp profiles: %v", err)
		return
	}
	if len(profiles) == 0 {
		log.Warningf("no dd snmp profiles found in '%s'", dir)
		return
	}

	log.Infof("found %d profiles in '%s'", len(profiles), dir)
	ddProfiles = profiles
}

func load(dirpath string) ([]*Profile, error) {
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

	for _, name := range prof.Definition.Extends {
		baseProf, err := loadProfile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}

		prof.merge(baseProf)
	}

	return &prof, nil
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}
