// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type isolatedCatalog struct {
	profileName string
	fileURL     string
}

func stageIsolatedCatalog(profilePath, dumpPath string) (isolatedCatalog, func(), error) {
	profileAbs, err := filepath.Abs(profilePath)
	if err != nil {
		return isolatedCatalog{}, nil, fmt.Errorf("resolve profile path: %w", err)
	}
	dumpAbs, err := filepath.Abs(dumpPath)
	if err != nil {
		return isolatedCatalog{}, nil, fmt.Errorf("resolve dump path: %w", err)
	}

	base := filepath.Base(profileAbs)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if !promprofiles.IsValidProfileName(name) {
		return isolatedCatalog{}, nil, fmt.Errorf(
			"profile basename %q must be lowercase letters, digits, or underscores and start with a letter",
			name,
		)
	}

	raw, err := os.ReadFile(profileAbs)
	if err != nil {
		return isolatedCatalog{}, nil, fmt.Errorf("read profile %q: %w", profilePath, err)
	}
	dumpRaw, err := os.ReadFile(dumpAbs)
	if err != nil {
		return isolatedCatalog{}, nil, fmt.Errorf("read dump %q: %w", dumpPath, err)
	}

	root, err := os.MkdirTemp("", "netdata-prometheus-profile-validation-")
	if err != nil {
		return isolatedCatalog{}, nil, fmt.Errorf("create isolated validation directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	fail := func(err error) (isolatedCatalog, func(), error) {
		cleanup()
		return isolatedCatalog{}, nil, err
	}

	userRoot := filepath.Join(root, "prefix", "etc", "netdata")
	userProfiles := filepath.Join(userRoot, "go.d", "prometheus.profiles")
	execDir := filepath.Join(root, "prefix", "usr", "libexec", "netdata", "plugins.d")
	stockProfiles := filepath.Join(
		root,
		"prefix",
		"usr",
		"lib",
		"netdata",
		"conf.d",
		"go.d",
		"prometheus.profiles",
		"default",
	)
	for _, dir := range []string{userProfiles, execDir, stockProfiles} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fail(fmt.Errorf("create isolated catalog directory %q: %w", dir, err))
		}
	}

	stagedProfile := filepath.Join(userProfiles, name+".yaml")
	if err := os.WriteFile(stagedProfile, raw, 0o600); err != nil {
		return fail(fmt.Errorf("stage candidate profile: %w", err))
	}
	stagedDump := filepath.Join(root, "metrics.txt")
	if err := os.WriteFile(stagedDump, dumpRaw, 0o600); err != nil {
		return fail(fmt.Errorf("snapshot evidence dump: %w", err))
	}

	// Preflight as stock because stock-profile errors are fatal. User catalog
	// errors are intentionally skipped by the runtime loader, which could
	// otherwise hide a broken candidate behind an empty catalog.
	preflight, err := promprofiles.LoadFromDirs([]promprofiles.DirSpec{{
		Path:    userProfiles,
		IsStock: true,
	}})
	if err != nil {
		return fail(fmt.Errorf("strict profile catalog preflight: %w", err))
	}
	profile, ok := preflight.Get(name)
	if !ok {
		return fail(fmt.Errorf("strict profile catalog preflight did not load %q", name))
	}
	if _, err := profile.Template(); err != nil {
		return fail(fmt.Errorf("strict profile template preflight: %w", err))
	}

	// DefaultCatalog is process-global. A validator process therefore validates
	// exactly one candidate and points that global discovery at this isolated
	// prefix before the collector first accesses it.
	executable.Name = "go.d"
	executable.Directory = execDir
	buildinfo.UserConfigDir = ""
	buildinfo.StockConfigDir = ""
	if err := os.Setenv("NETDATA_USER_CONFIG_DIR", ""); err != nil {
		return fail(fmt.Errorf("isolate user config environment: %w", err))
	}
	if err := os.Setenv("NETDATA_STOCK_CONFIG_DIR", ""); err != nil {
		return fail(fmt.Errorf("isolate stock config environment: %w", err))
	}
	pluginconfig.MustInit(pluginconfig.InitInput{ConfDir: []string{userRoot}})

	catalog, err := promprofiles.DefaultCatalog()
	if err != nil {
		return fail(fmt.Errorf("load isolated runtime profile catalog: %w", err))
	}
	if _, ok := catalog.Get(name); !ok {
		return fail(fmt.Errorf("isolated runtime profile catalog did not load candidate %q", name))
	}

	return isolatedCatalog{
		profileName: name,
		fileURL:     (&url.URL{Scheme: "file", Path: filepath.ToSlash(stagedDump)}).String(),
	}, cleanup, nil
}
