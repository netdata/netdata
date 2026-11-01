// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"errors"
	"path/filepath"
	"time"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
)

// RuntimeConfig is administrator-owned process configuration, never monitor input.
// Protected preloads are borrowed for the process lifetime and are outside cache GC.
type RuntimeConfig struct {
	Mode                  string `yaml:"mode" json:"mode"`
	CachePath             string `yaml:"cache_path" json:"cache_path,omitempty"`
	MaxBytes              int64  `yaml:"max_bytes" json:"max_bytes"`
	PreviousRetentionDays int    `yaml:"previous_retention_days" json:"previous_retention_days"`
	PreloadPath           string `yaml:"preload_path" json:"preload_path,omitempty"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Mode: "managed", MaxBytes: runtime.DefaultMaxBytes, PreviousRetentionDays: 7}
}

func (c RuntimeConfig) validate() error {
	if c.Mode != "managed" && c.Mode != "preloaded" {
		return errors.New("mode must be managed or preloaded")
	}
	if c.MaxBytes <= 0 {
		return errors.New("max_bytes must be positive")
	}
	if c.PreviousRetentionDays < 0 || c.PreviousRetentionDays > 36500 {
		return errors.New("previous_retention_days must be between 0 and 36500")
	}
	if c.Mode == "managed" && c.PreloadPath != "" {
		return errors.New("preload_path requires mode: preloaded")
	}
	if c.Mode == "preloaded" && c.CachePath != "" {
		return errors.New("cache_path is only valid in managed mode")
	}
	return nil
}

// StoreConfig resolves only explicit installation paths; debug mode cannot create
// an implicit persistent cache in the working directory or temporary storage.
func (c RuntimeConfig) StoreConfig(varLibPath string) (runtime.Config, error) {
	path := c.CachePath
	if path == "" {
		if !filepath.IsAbs(varLibPath) {
			return runtime.Config{}, errors.New("runtime cache requires an absolute NETDATA_LIB_DIR or runtime.cache_path")
		}
		path = filepath.Join(varLibPath, "dem", "runtime")
	}
	if !filepath.IsAbs(path) {
		return runtime.Config{}, errors.New("runtime.cache_path must be absolute")
	}
	return runtime.Config{Path: path, MaxBytes: c.MaxBytes,
		PreviousRetention: time.Duration(c.PreviousRetentionDays) * 24 * time.Hour}, nil
}
