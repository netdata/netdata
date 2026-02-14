// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "fmt"

type engineConfig struct {
	autogen AutogenPolicy
}

// Option mutates engine configuration at construction time.
type Option func(*engineConfig) error

const (
	defaultMaxTypeIDLen = 1200
)

// AutogenPolicy controls unmatched-series fallback chart generation.
type AutogenPolicy struct {
	Enabled bool

	// TypeID is the chart-type prefix used by Netdata runtime checks
	// (`type.id` length guard). Typically this is `<plugin>.<job>`.
	TypeID string
	// MaxTypeIDLen is the max allowed full `type.id` length.
	// Zero means default (1200).
	MaxTypeIDLen int
	// ExpireAfterSuccessCycles controls autogen chart/dimension expiry on
	// successful collection cycles where the series is not seen.
	// Zero disables expiry.
	ExpireAfterSuccessCycles uint64
}

func defaultAutogenPolicy() AutogenPolicy {
	return AutogenPolicy{
		Enabled:      false,
		MaxTypeIDLen: defaultMaxTypeIDLen,
	}
}

// WithAutogenPolicy configures unmatched-series autogen behavior.
func WithAutogenPolicy(policy AutogenPolicy) Option {
	return func(cfg *engineConfig) error {
		maxLen := policy.MaxTypeIDLen
		if maxLen <= 0 {
			maxLen = defaultMaxTypeIDLen
		}
		if maxLen < 4 {
			return fmt.Errorf("autogen max type.id len must be >= 4, got %d", maxLen)
		}
		policy.MaxTypeIDLen = maxLen
		cfg.autogen = policy
		return nil
	}
}

func applyOptions(opts ...Option) (engineConfig, error) {
	cfg := engineConfig{
		autogen: defaultAutogenPolicy(),
	}
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
