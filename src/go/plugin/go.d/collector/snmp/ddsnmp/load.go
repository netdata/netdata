// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

func load(dirpath string) ([]*Profile, error) {
	var profiles []*Profile

	if err := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			return nil
		}
		profile, err := loadYAML(path)
		if err != nil {
			return err
		}
		profiles = append(profiles, profile)
		return nil
	}); err != nil {
		return nil, err
	}

	return profiles, nil
}

func loadYAML(filename string) (*Profile, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var prof Profile
	if err := yaml.Unmarshal(content, &prof); err != nil {
		return nil, err
	}

	if prof.SourceFile == "" {
		prof.SourceFile, _ = filepath.Abs(filename)
	}

	dir := filepath.Dir(filename)

	for _, name := range prof.Extends {
		baseProf, err := loadYAML(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		mergeProfiles(&prof, baseProf)
	}

	return &prof, nil
}

func mergeProfiles(child, parent *Profile) {
	child.Metrics = append(parent.Metrics, child.Metrics...)
	//
	//if child.Metadata == nil || len(child.Metadata.Device) == 0 {
	//	return
	//}
	//if child.Metadata.Device.Fields == nil {
	//	child.Metadata.Device.Fields = make(map[string]Symbol)
	//}
	//
	//for key, value := range parent.Metadata.Device.Fields {
	//	if _, exists := child.Metadata.Device.Fields[key]; !exists {
	//		child.Metadata.Device.Fields[key] = value
	//	}
	//}
}
