// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
)

// RuntimeConfig is administrator-owned process configuration, never monitor input.
// Protected preloads are borrowed for the process lifetime and are outside cache GC.
type RuntimeConfig struct {
	Mode                  string `yaml:"mode" json:"mode"`
	AllowDownload         bool   `yaml:"allow_download" json:"allow_download"`
	CachePath             string `yaml:"cache_path" json:"cache_path,omitempty"`
	ManifestPath          string `yaml:"manifest_path" json:"manifest_path,omitempty"`
	MaxBytes              int64  `yaml:"max_bytes" json:"max_bytes"`
	PreviousRetentionDays int    `yaml:"previous_retention_days" json:"previous_retention_days"`
	IdleRetentionDays     int    `yaml:"idle_retention_days" json:"idle_retention_days"`
	PreloadPath           string `yaml:"preload_path" json:"preload_path,omitempty"`
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Mode: "managed", AllowDownload: true, MaxBytes: runtime.DefaultMaxBytes,
		PreviousRetentionDays: 7, IdleRetentionDays: 30}
}

func (c RuntimeConfig) validate() error {
	if c.Mode != "managed" && c.Mode != "preloaded" {
		return errors.New("mode must be managed or preloaded")
	}
	if c.MaxBytes <= 0 {
		return errors.New("max_bytes must be positive")
	}
	for name, days := range map[string]int{"previous_retention_days": c.PreviousRetentionDays, "idle_retention_days": c.IdleRetentionDays} {
		if days < 0 || days > 36500 {
			return fmt.Errorf("%s must be between 0 and 36500", name)
		}
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
		PreviousRetention: time.Duration(c.PreviousRetentionDays) * 24 * time.Hour,
		IdleRetention:     time.Duration(c.IdleRetentionDays) * 24 * time.Hour}, nil
}

// UnmarshalYAML rejects unsupported runtime settings while leaving framework
// settings in dem.conf to their existing owner. Preserve defaults on partial input.
func (c *RuntimeConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type plain RuntimeConfig
	value := plain(*c)
	// Decode types against the original input so semantic errors retain real lines.
	if err := unmarshal(&value); err != nil {
		return err
	}
	var fields yaml.MapSlice
	if err := unmarshal(&fields); err != nil {
		return err
	}
	known := make(map[string]bool)
	typ := reflect.TypeOf(value)
	for i := 0; i < typ.NumField(); i++ {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("yaml"), ",")
		known[name] = true
	}
	seen := make(map[string]bool)
	for _, field := range fields {
		name, ok := field.Key.(string)
		if !ok {
			return errors.New("runtime: setting names must be strings")
		}
		if !known[name] {
			return fmt.Errorf("runtime: unknown setting %q", name)
		}
		if seen[name] {
			return fmt.Errorf("runtime: duplicate setting %q", name)
		}
		seen[name] = true
	}
	*c = RuntimeConfig(value)
	return nil
}
