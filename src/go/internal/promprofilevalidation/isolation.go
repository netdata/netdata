// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type isolatedCatalog struct {
	profileName string
	fileURL     string
	catalog     promprofiles.Catalog
	root        string
	dumpRaw     []byte
}

func (i isolatedCatalog) stageFutureInputs(inputs []futureInput) (string, error) {
	combined, err := appendFutureInputs(i.dumpRaw, inputs)
	if err != nil {
		return "", err
	}
	path := filepath.Join(i.root, "metrics-with-future-inputs.txt")
	if err := os.WriteFile(path, combined, 0o600); err != nil {
		return "", fmt.Errorf("stage future-input exposition: %w", err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String(), nil
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

	profilesDir := filepath.Join(root, "profiles")
	if err := os.MkdirAll(profilesDir, 0o700); err != nil {
		return fail(fmt.Errorf("create isolated catalog directory %q: %w", profilesDir, err))
	}

	stagedProfile := filepath.Join(profilesDir, name+".yaml")
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
		Path:    profilesDir,
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

	return isolatedCatalog{
		profileName: name,
		fileURL:     (&url.URL{Scheme: "file", Path: filepath.ToSlash(stagedDump)}).String(),
		catalog:     preflight,
		root:        root,
		dumpRaw:     dumpRaw,
	}, cleanup, nil
}
