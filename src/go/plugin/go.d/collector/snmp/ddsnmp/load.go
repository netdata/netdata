// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/logger"
)

var log = logger.New().With("component", "snmp/ddsnmp")

func load(dirpath string) ([]*Profile, error) {
	var profiles []*Profile

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
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
