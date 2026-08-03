// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	defaultWindow     = 5 * time.Second
	defaultMaxEntries = 100000
)

type Config struct {
	Enabled         bool
	WindowSec       int
	CacheMaxEntries int
	KeyVarbinds     []string
}

type Policy struct {
	enabled     bool
	window      time.Duration
	maxEntries  int
	keyVarbinds []string
}

func Normalize(cfg Config) (Policy, error) {
	policy := Policy{enabled: cfg.Enabled}
	if !cfg.Enabled {
		return policy, nil
	}
	if cfg.WindowSec < 0 {
		return Policy{}, fmt.Errorf("dedup.window_sec must be non-negative, got %d", cfg.WindowSec)
	}
	if cfg.CacheMaxEntries < 0 {
		return Policy{}, fmt.Errorf("dedup.cache_max_entries must be non-negative, got %d", cfg.CacheMaxEntries)
	}
	for i, key := range cfg.KeyVarbinds {
		if strings.TrimSpace(key) == "" {
			return Policy{}, fmt.Errorf("dedup.key_varbinds[%d] must not be empty", i)
		}
	}

	policy.window = time.Duration(cfg.WindowSec) * time.Second
	if policy.window <= 0 {
		policy.window = defaultWindow
	}
	policy.maxEntries = cfg.CacheMaxEntries
	if policy.maxEntries <= 0 {
		policy.maxEntries = defaultMaxEntries
	}
	policy.keyVarbinds = slices.Clone(cfg.KeyVarbinds)
	return policy, nil
}

func (p Policy) Enabled() bool { return p.enabled }

// KeyVarbinds returns a copy of the normalized job-level key names. Callers
// should retain the result rather than cloning it on each admission.
func (p Policy) KeyVarbinds() []string { return slices.Clone(p.keyVarbinds) }
