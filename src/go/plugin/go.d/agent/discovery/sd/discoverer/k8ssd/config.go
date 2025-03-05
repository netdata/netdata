// SPDX-License-Identifier: GPL-3.0-or-later

package k8ssd

import (
	"errors"
	"fmt"
)

type Config struct {
	APIServer  string   `yaml:"api_server"` // TODO: not used
	Role       string   `yaml:"role"`
	Tags       string   `yaml:"tags"`
	Namespaces []string `yaml:"namespaces"`
	Selector   struct {
		Label string `yaml:"label"`
		Field string `yaml:"field"`
	} `yaml:"selector"`
	Pod struct {
		LocalMode bool `yaml:"local_mode"`
	} `yaml:"pod"`
}

func validateConfig(cfg Config) error {
	switch role(cfg.Role) {
	case rolePod, roleService:
	default:
		return fmt.Errorf("unknown role: '%s'", cfg.Role)
	}
	if cfg.Tags == "" {
		return errors.New("'tags' not set")
	}
	return nil
}
