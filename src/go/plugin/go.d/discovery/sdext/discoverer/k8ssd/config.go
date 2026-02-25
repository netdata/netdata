// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"fmt"
)

type Config struct {
	Source string `yaml:"-" json:"-"`

	APIServer  string   `yaml:"api_server,omitempty" json:"-"` // TODO: not used
	Role       string   `yaml:"role,omitempty" json:"role,omitempty"`
	Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
	Selector   struct {
		Label string `yaml:"label,omitempty" json:"label,omitempty"`
		Field string `yaml:"field,omitempty" json:"field,omitempty"`
	} `yaml:"selector,omitempty" json:"selector,omitempty"`
	Pod struct {
		LocalMode bool `yaml:"local_mode,omitempty" json:"local_mode,omitempty"`
	} `yaml:"pod,omitempty" json:"pod,omitempty"`
}

func validateConfig(cfg Config) error {
	switch role(cfg.Role) {
	case rolePod, roleService:
	default:
		return fmt.Errorf("unknown role: '%s'", cfg.Role)
	}
	return nil
}
