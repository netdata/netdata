// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
)

// RuntimeConfig is administrator-owned process configuration, never monitor input.
// External paths are borrowed for the process lifetime and are outside cache GC.
type RuntimeConfig struct {
	Mode                  string `yaml:"mode" json:"mode"`
	AllowDownload         bool   `yaml:"allow_download" json:"allow_download"`
	CachePath             string `yaml:"cache_path" json:"cache_path,omitempty"`
	ManifestPath          string `yaml:"manifest_path" json:"manifest_path,omitempty"`
	MaxBytes              int64  `yaml:"max_bytes" json:"max_bytes"`
	PreviousRetentionDays int    `yaml:"previous_retention_days" json:"previous_retention_days"`
	IdleRetentionDays     int    `yaml:"idle_retention_days" json:"idle_retention_days"`
	NodePath              string `yaml:"node_path" json:"node_path,omitempty"`
	DependenciesPath      string `yaml:"dependencies_path" json:"dependencies_path,omitempty"`
	BrowserPath           string `yaml:"browser_path" json:"browser_path,omitempty"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Mode: "managed", AllowDownload: true, MaxBytes: runtime.DefaultMaxBytes,
		PreviousRetentionDays: 7, IdleRetentionDays: 30}
}

func (c RuntimeConfig) validate() error {
	if c.Mode != "managed" && c.Mode != "external" {
		return errors.New("mode must be managed or external")
	}
	if c.MaxBytes <= 0 {
		return errors.New("max_bytes must be positive")
	}
	for name, days := range map[string]int{"previous_retention_days": c.PreviousRetentionDays, "idle_retention_days": c.IdleRetentionDays} {
		if days < 0 || days > 36500 {
			return fmt.Errorf("%s must be between 0 and 36500", name)
		}
	}
	if c.Mode == "managed" && (c.NodePath != "" || c.DependenciesPath != "" || c.BrowserPath != "") {
		return errors.New("explicit executable paths require mode: external")
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
		PreviousRetention: time.Duration(c.PreviousRetentionDays) * 24 * time.Hour,
		IdleRetention:     time.Duration(c.IdleRetentionDays) * 24 * time.Hour}, nil
}
