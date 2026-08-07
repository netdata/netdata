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

type stagedValidationInputs struct {
	profileName  string
	profileNames []string
	profiles     []promprofiles.Profile
	fileURL      string
	catalog      promprofiles.Catalog
	root         string
	dumpRaw      []byte
}

func (i stagedValidationInputs) stageFutureInputs(inputs []futureInput) (string, error) {
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

func stageValidationInputs(profilePath string, supportingProfilePaths []string, dumpPath string) (stagedValidationInputs, func(), error) {
	dumpAbs, err := filepath.Abs(dumpPath)
	if err != nil {
		return stagedValidationInputs{}, nil, fmt.Errorf("resolve dump path: %w", err)
	}
	dumpRaw, err := os.ReadFile(dumpAbs)
	if err != nil {
		return stagedValidationInputs{}, nil, fmt.Errorf("read dump %q: %w", dumpPath, err)
	}

	root, err := os.MkdirTemp("", "netdata-prometheus-profile-validation-")
	if err != nil {
		return stagedValidationInputs{}, nil, fmt.Errorf("create isolated validation directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	fail := func(err error) (stagedValidationInputs, func(), error) {
		cleanup()
		return stagedValidationInputs{}, nil, err
	}

	profilesDir := filepath.Join(root, "profiles")
	if err := os.MkdirAll(profilesDir, 0o700); err != nil {
		return fail(fmt.Errorf("create isolated catalog directory %q: %w", profilesDir, err))
	}

	profilePaths := append([]string{profilePath}, supportingProfilePaths...)
	profileNames := make([]string, 0, len(profilePaths))
	seenNames := make(map[string]string, len(profilePaths))
	for index, path := range profilePaths {
		profileAbs, err := filepath.Abs(path)
		if err != nil {
			return fail(fmt.Errorf("resolve profile path %q: %w", path, err))
		}
		base := filepath.Base(profileAbs)
		name := strings.TrimSuffix(base, filepath.Ext(base))
		if !promprofiles.IsValidProfileName(name) {
			return fail(fmt.Errorf(
				"profile basename %q must be lowercase letters, digits, or underscores and start with a letter",
				name,
			))
		}
		if first, ok := seenNames[name]; ok {
			return fail(fmt.Errorf("profile identity %q is duplicated by %q and %q", name, first, path))
		}
		seenNames[name] = path

		raw, err := os.ReadFile(profileAbs)
		if err != nil {
			return fail(fmt.Errorf("read profile %q: %w", path, err))
		}
		if err := os.WriteFile(filepath.Join(profilesDir, name+".yaml"), raw, 0o600); err != nil {
			role := "supporting"
			if index == 0 {
				role = "candidate"
			}
			return fail(fmt.Errorf("stage %s profile %q: %w", role, path, err))
		}
		profileNames = append(profileNames, name)
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
	profiles := make([]promprofiles.Profile, 0, len(profileNames))
	for _, name := range profileNames {
		profile, ok := preflight.Get(name)
		if !ok {
			return fail(fmt.Errorf("strict profile catalog preflight did not load %q", name))
		}
		if _, err := profile.Template(); err != nil {
			return fail(fmt.Errorf("strict profile template preflight for %q: %w", name, err))
		}
		profiles = append(profiles, profile)
	}

	return stagedValidationInputs{
		profileName:  profileNames[0],
		profileNames: profileNames,
		profiles:     profiles,
		fileURL:      (&url.URL{Scheme: "file", Path: filepath.ToSlash(stagedDump)}).String(),
		catalog:      preflight,
		root:         root,
		dumpRaw:      dumpRaw,
	}, cleanup, nil
}
