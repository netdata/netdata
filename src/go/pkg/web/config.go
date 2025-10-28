// SPDX-License-Identifier: GPL-3.0-or-later

package web

// HTTPConfig is a struct with embedded RequestConfig and ClientConfig.
// This structure intended to be part of the module configuration.
// Supported configuration file formats: YAML.
type HTTPConfig struct {
	RequestConfig `yaml:",inline" json:""`
	ClientConfig  `yaml:",inline" json:""`
}
