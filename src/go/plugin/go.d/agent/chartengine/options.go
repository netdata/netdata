// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "fmt"

type engineConfig struct {
	// Reserved for future engine-wide knobs.
}

// Option mutates engine configuration at construction time.
type Option func(*engineConfig) error

func applyOptions(opts ...Option) (engineConfig, error) {
	cfg := engineConfig{}
	for i, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return engineConfig{}, fmt.Errorf("chartengine: option[%d]: %w", i, err)
		}
	}
	return cfg, nil
}
