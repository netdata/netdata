// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
)

// Mode selects who owns the browser runtime files.
type Mode string

const (
	// ModeManaged runs explicitly installed releases from the DEM-managed cache.
	ModeManaged Mode = "managed"
	// ModeNative runs the package-manager-owned netdata-dem-runtime payload.
	ModeNative Mode = "native"
	// ModePreloaded runs an administrator-sealed preload at its final root.
	ModePreloaded Mode = "preloaded"
)

// DefaultMode is set to native by DEB/RPM builds and preloaded by complete DEM images.
// DefaultMode and DefaultPreloadPath are strings for the linker's -X flag.
var DefaultMode = string(ModeManaged)

// DefaultPreloadPath is set by complete DEM images to the root of the bundled runtime.
var DefaultPreloadPath string

const day = 24 * time.Hour

// Config is administrator-owned process configuration, never monitor input.
// Protected preloads are borrowed for the process lifetime and are outside cache GC.
type Config struct {
	Mode                  Mode   `yaml:"mode"`
	CachePath             string `yaml:"cache_path"`
	MaxBytes              int64  `yaml:"max_bytes"`
	PreviousRetentionDays int    `yaml:"previous_retention_days"`
	PreloadPath           string `yaml:"preload_path"`
	ReleaseURL            string `yaml:"release_url"`
	OfflineKeyring        string `yaml:"offline_keyring"`
}

// DefaultConfig takes the store allowance and previous-generation grace from the runtime store defaults.
func DefaultConfig() Config {
	cfg := Config{
		Mode:                  Mode(DefaultMode),
		ReleaseURL:            defaultReleaseURL(AgentRelease(), runtime.GOARCH),
		MaxBytes:              browserruntime.DefaultMaxBytes,
		PreviousRetentionDays: int(browserruntime.DefaultPreviousRetention / day),
	}
	if cfg.Mode == ModePreloaded {
		cfg.PreloadPath = DefaultPreloadPath
	}
	return cfg
}

// AgentRelease is the canonical build identity, independent of display/native
// package version encodings. Unstamped developer builds have no public default.
func AgentRelease() browserruntime.AgentRelease {
	return browserruntime.AgentRelease{
		Version: buildinfo.ReleaseVersion,
		Channel: buildinfo.ReleaseChannel,
		Source:  buildinfo.ReleaseRef,
	}
}

func defaultReleaseURL(agent browserruntime.AgentRelease, arch string) string {
	if agent.Validate() != nil || (arch != "amd64" && arch != "arm64") {
		return ""
	}
	return "https://artifacts.netdata.cloud/dem-runtime/" + agent.Channel + "/" + agent.Version + "/" + arch + "/release.json"
}

// NativeConfig uses the same exact pair requirement as managed and preload modes.
func NativeConfig() browserruntime.NativeConfig {
	config := browserruntime.DefaultNativeConfig()
	config.Agent = new(AgentRelease())
	return config
}

func (c Config) Validate() error {
	switch c.Mode {
	case ModeManaged, ModeNative, ModePreloaded:
	default:
		return errors.New("mode must be managed, native or preloaded")
	}
	if c.MaxBytes <= 0 {
		return errors.New("max_bytes must be positive")
	}
	if c.PreviousRetentionDays < 0 || c.PreviousRetentionDays > 36500 {
		return errors.New("previous_retention_days must be between 0 and 36500")
	}
	if c.Mode != ModePreloaded && c.PreloadPath != "" {
		return errors.New("preload_path requires mode: preloaded")
	}
	if c.Mode != ModeManaged && c.CachePath != "" {
		return errors.New("cache_path is only valid in managed mode")
	}
	if c.ReleaseURL != "" && !browserruntime.ValidHTTPSURL(c.ReleaseURL) {
		return errors.New("release_url requires HTTPS without credentials or a fragment")
	}
	if c.OfflineKeyring != "" && !filepath.IsAbs(c.OfflineKeyring) {
		return errors.New("offline_keyring requires an absolute path to a public OpenPGP keyring")
	}
	return nil
}

// ReleaseLoader selects canonical online trust only with network consent.
// Offline imports require an explicit administrator-provisioned public keyring.
// Call it under installation exclusion, never before checking update consent.
func (c Config) ReleaseLoader(
	path string,
	networkAllowed bool,
) func(context.Context) (browserruntime.VerifiedRelease, error) {
	return func(ctx context.Context) (browserruntime.VerifiedRelease, error) {
		if path == "" {
			if !networkAllowed {
				return browserruntime.VerifiedRelease{}, errors.New(
					"release download requires explicit network consent",
				)
			}
			if c.ReleaseURL == "" {
				return browserruntime.VerifiedRelease{}, errors.New(
					"online runtime preparation requires runtime.release_url",
				)
			}
		}
		var keys *browserruntime.ReleaseTrust
		var trustErr error
		if networkAllowed {
			keys, trustErr = browserruntime.FetchReleaseKeyring(ctx)
		} else {
			if c.OfflineKeyring == "" {
				return browserruntime.VerifiedRelease{}, errors.New("offline preparation requires runtime.offline_keyring; no runtime release verification key is configured")
			}
			keys, trustErr = browserruntime.ReadReleaseKeyring(c.OfflineKeyring)
		}
		if trustErr != nil {
			return browserruntime.VerifiedRelease{}, fmt.Errorf("runtime verification key: %w", trustErr)
		}
		var release browserruntime.VerifiedRelease
		var err error
		if path == "" {
			release, err = browserruntime.FetchRelease(ctx, c.ReleaseURL, keys)
		} else {
			release, err = browserruntime.ReadRelease(path, keys)
		}
		if err != nil {
			return browserruntime.VerifiedRelease{}, err
		}
		if !release.Release().Manifest.MatchesHost() {
			return browserruntime.VerifiedRelease{}, errors.New("runtime release does not match this architecture")
		}
		if err := release.Release().ValidateForAgent(AgentRelease()); err != nil {
			return browserruntime.VerifiedRelease{}, err
		}
		return release, nil
	}
}

// StoreConfig resolves only explicit installation paths; debug mode cannot create
// an implicit persistent cache in the working directory or temporary storage.
func (c Config) StoreConfig(varLibPath string) (browserruntime.Config, error) {
	path := c.CachePath
	if path == "" {
		if !filepath.IsAbs(varLibPath) {
			return browserruntime.Config{}, errors.New(
				"runtime cache requires an absolute NETDATA_LIB_DIR or runtime.cache_path",
			)
		}
		path = filepath.Join(varLibPath, "dem", "runtime")
	}
	if !filepath.IsAbs(path) {
		return browserruntime.Config{}, errors.New("runtime.cache_path must be absolute")
	}
	return browserruntime.Config{
		Path:              path,
		MaxBytes:          c.MaxBytes,
		PreviousRetention: time.Duration(c.PreviousRetentionDays) * day,
		Agent:             new(AgentRelease()),
	}, nil
}
