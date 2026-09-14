// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

const (
	defaultWindow     = 5 * time.Second
	defaultMaxEntries = 100000
	// MaxWindowSec is the largest whole-second window representable by time.Duration.
	MaxWindowSec = int64(math.MaxInt64) / int64(time.Second)
)

type Config struct {
	Enabled         bool
	WindowSec       int64
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
	if cfg.WindowSec > MaxWindowSec {
		return Policy{}, fmt.Errorf("dedup.window_sec must not exceed %d, got %d", MaxWindowSec, cfg.WindowSec)
	}
	if cfg.CacheMaxEntries < 0 {
		return Policy{}, fmt.Errorf("dedup.cache_max_entries must be non-negative, got %d", cfg.CacheMaxEntries)
	}
	keyVarbinds := make([]string, len(cfg.KeyVarbinds))
	for i, key := range cfg.KeyVarbinds {
		key = strings.TrimSpace(key)
		if key == "" {
			return Policy{}, fmt.Errorf("dedup.key_varbinds[%d] must not be empty", i)
		}
		keyVarbinds[i] = key
	}

	policy.window = time.Duration(cfg.WindowSec) * time.Second
	if policy.window <= 0 {
		policy.window = defaultWindow
	}
	policy.maxEntries = cfg.CacheMaxEntries
	if policy.maxEntries <= 0 {
		policy.maxEntries = defaultMaxEntries
	}
	policy.keyVarbinds = keyVarbinds
	return policy, nil
}

func (p Policy) Enabled() bool { return p.enabled }

// KeyVarbinds returns a copy of the normalized job-level key names. Callers
// should retain the result rather than cloning it on each admission.
func (p Policy) KeyVarbinds() []string { return slices.Clone(p.keyVarbinds) }
