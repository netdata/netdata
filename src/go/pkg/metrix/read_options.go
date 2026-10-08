// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// ReadOption controls reader visibility mode.
type ReadOption interface {
	readOption()
}

// Options are values resolved by type, so resolving them does not move the
// read configuration to the heap.
type (
	readRawOption       struct{}
	readFlattenOption   struct{}
	readHostScopeOption struct{ scopeKey string }
)

func (readRawOption) readOption()       {}
func (readFlattenOption) readOption()   {}
func (readHostScopeOption) readOption() {}

type readConfig struct {
	raw          bool
	flatten      bool
	hostScopeKey string
}

func resolveReadConfig(opts ...ReadOption) readConfig {
	cfg := readConfig{}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case readRawOption:
			cfg.raw = true
		case readFlattenOption:
			cfg.flatten = true
		case readHostScopeOption:
			cfg.hostScopeKey = opt.scopeKey
		}
	}
	return cfg
}

// ReadRaw enables raw committed-series visibility mode for Read().
// Without this option, Read() applies freshness filtering.
func ReadRaw() ReadOption {
	return readRawOption{}
}

// ReadFlatten enables flattened scalar-series view mode for Read().
// Without this option, Read() returns canonical typed-family view.
func ReadFlatten() ReadOption {
	return readFlattenOption{}
}

// ReadHostScope filters the reader to one host scope. The empty key is the
// default scope and matches unscoped writes.
func ReadHostScope(scopeKey string) ReadOption {
	return readHostScopeOption{scopeKey: scopeKey}
}
