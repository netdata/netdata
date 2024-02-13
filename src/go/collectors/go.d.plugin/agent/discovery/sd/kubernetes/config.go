// SPDX-License-Identifier: GPL-3.0-or-later

package kubernetes

import "errors"

type Config struct {
	APIServer  string         `yaml:"api_server"` // TODO: not used
	Namespaces []string       `yaml:"namespaces"`
	Pod        *PodConfig     `yaml:"pod"`
	Service    *ServiceConfig `yaml:"service"`
}

type PodConfig struct {
	Tags      string `yaml:"tags"`
	LocalMode bool   `yaml:"local_mode"`
	Selector  struct {
		Label string `yaml:"label"`
		Field string `yaml:"field"`
	} `yaml:"selector"`
}

type ServiceConfig struct {
	Tags     string `yaml:"tags"`
	Selector struct {
		Label string `yaml:"label"`
		Field string `yaml:"field"`
	} `yaml:"selector"`
}

func validateConfig(cfg Config) error {
	if cfg.Pod == nil && cfg.Service == nil {
		return errors.New("no discoverers configured")
	}

	return nil
}
