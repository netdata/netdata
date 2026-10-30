// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	goruntime "runtime"
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
	ReleaseURL            string `yaml:"release_url" json:"release_url,omitempty"`
	OfflineKeyring        string `yaml:"offline_keyring" json:"offline_keyring,omitempty"`
}

// DefaultRuntimeMode is set to native by DEB/RPM builds. Other installations use
// explicitly installed managed bundles unless an administrator selects a preload.
var DefaultRuntimeMode = "managed"

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{Mode: DefaultRuntimeMode, MaxBytes: runtime.DefaultMaxBytes, PreviousRetentionDays: 7}
}

func (c RuntimeConfig) validate() error {
	if c.Mode != "managed" && c.Mode != "preloaded" && c.Mode != "native" {
		return errors.New("mode must be managed, native or preloaded")
	}
	if c.MaxBytes <= 0 {
		return errors.New("max_bytes must be positive")
	}
	if c.PreviousRetentionDays < 0 || c.PreviousRetentionDays > 36500 {
		return errors.New("previous_retention_days must be between 0 and 36500")
	}
	if c.Mode != "preloaded" && c.PreloadPath != "" {
		return errors.New("preload_path requires mode: preloaded")
	}
	if c.Mode != "managed" && c.CachePath != "" {
		return errors.New("cache_path is only valid in managed mode")
	}
	if c.ReleaseURL != "" {
		u, err := url.Parse(c.ReleaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("release_url requires HTTPS without credentials or a fragment")
		}
	}
	if c.OfflineKeyring != "" && !filepath.IsAbs(c.OfflineKeyring) {
		return errors.New("offline_keyring requires an absolute path to a public OpenPGP keyring")
	}
	return nil
}

// ReleaseLoader selects canonical online trust only with network consent.
// Offline imports require an explicit administrator-provisioned public keyring.
// Call it under installation exclusion, never before checking update consent.
func (c RuntimeConfig) ReleaseLoader(path string, networkAllowed bool) func(context.Context) (runtime.Release, error) {
	return func(ctx context.Context) (runtime.Release, error) {
		var keys *runtime.ReleaseTrust
		var trustErr error
		if networkAllowed {
			keys, trustErr = runtime.FetchReleaseKeyring(ctx)
		} else {
			if c.OfflineKeyring == "" {
				return runtime.Release{}, errors.New("offline preparation requires runtime.offline_keyring; no runtime release verification key is configured")
			}
			keys, trustErr = runtime.ReadReleaseKeyring(c.OfflineKeyring)
		}
		if trustErr != nil {
			return runtime.Release{}, fmt.Errorf("runtime verification key: %w", trustErr)
		}
		var release runtime.Release
		var err error
		if path == "" {
			if !networkAllowed {
				return runtime.Release{}, errors.New("release download requires explicit network consent")
			}
			release, err = runtime.FetchRelease(ctx, c.ReleaseURL, keys)
		} else {
			release, err = runtime.ReadRelease(path, keys)
		}
		if err != nil {
			return runtime.Release{}, err
		}
		if release.Manifest.Platform != "linux/"+goruntime.GOARCH {
			return runtime.Release{}, errors.New("runtime release does not match this architecture")
		}
		return release, nil
	}
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
