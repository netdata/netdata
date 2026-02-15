// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// ReadOption controls reader visibility mode.
type ReadOption interface {
	applyRead(*readConfig)
}

type readOptionFunc func(*readConfig)

func (f readOptionFunc) applyRead(cfg *readConfig) {
	f(cfg)
}

type readConfig struct {
	raw     bool
	flatten bool
}

func resolveReadConfig(opts ...ReadOption) readConfig {
	cfg := readConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyRead(&cfg)
		}
	}
	return cfg
}

// ReadRaw enables raw committed-series visibility mode for Read().
// Without this option, Read() applies freshness filtering.
func ReadRaw() ReadOption {
	return readOptionFunc(func(cfg *readConfig) {
		cfg.raw = true
	})
}

// ReadFlatten enables flattened scalar-series view mode for Read().
// Without this option, Read() returns canonical typed-family view.
func ReadFlatten() ReadOption {
	return readOptionFunc(func(cfg *readConfig) {
		cfg.flatten = true
	})
}
