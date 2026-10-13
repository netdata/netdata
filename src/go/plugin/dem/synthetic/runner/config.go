// SPDX-License-Identifier: GPL-3.0-or-later

package runner

type Config struct {
	NodePath         string `yaml:"node_path"         json:"node_path"`
	DependenciesPath string `yaml:"dependencies_path" json:"dependencies_path"`
	BrowserPath      string `yaml:"browser_path"      json:"browser_path"`
	AssetsPath       string `yaml:"-"                 json:"-"`
}
