// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	dumpPath     string
	dumpRaw      []byte
}

type profileInputError struct {
	index int
	code  string
	err   error
}

func (e *profileInputError) Error() string { return e.err.Error() }
func (e *profileInputError) Unwrap() error { return e.err }

func (i *stagedValidationInputs) replaceDump(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read dump %q: %w", path, err)
	}
	if err := os.WriteFile(i.dumpPath, raw, 0o600); err != nil {
		return fmt.Errorf("replace staged evidence dump: %w", err)
	}
	i.dumpRaw = raw
	return nil
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

	profilePaths := append([]string{profilePath}, supportingProfilePaths...)
	profileNames := make([]string, 0, len(profilePaths))
	profileDirs := make([]promprofiles.DirSpec, 0, len(profilePaths))
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
			return fail(&profileInputError{index: index, code: "profile_load", err: fmt.Errorf("read profile %q: %w", path, err)})
		}
		profileDir := filepath.Join(root, "profiles", name)
		if err := os.MkdirAll(profileDir, 0o700); err != nil {
			return fail(&profileInputError{index: index, code: "profile_load", err: fmt.Errorf("create isolated profile directory: %w", err)})
		}
		if err := os.WriteFile(filepath.Join(profileDir, name+".yaml"), raw, 0o600); err != nil {
			role := "supporting"
			if index == 0 {
				role = "candidate"
			}
			return fail(&profileInputError{index: index, code: "profile_load", err: fmt.Errorf("stage %s profile %q: %w", role, path, err)})
		}
		profileNames = append(profileNames, name)
		profileDirs = append(profileDirs, promprofiles.DirSpec{Path: profileDir, IsStock: true})
	}
	stagedDump := filepath.Join(root, "metrics.txt")
	if err := os.WriteFile(stagedDump, dumpRaw, 0o600); err != nil {
		return fail(fmt.Errorf("snapshot evidence dump: %w", err))
	}

	// Preflight as stock because stock-profile errors are fatal. User catalog
	// errors are intentionally skipped by the runtime loader, which could
	// otherwise hide a broken candidate behind an empty catalog.
	catalogDirs := slices.Clone(profileDirs)
	slices.SortFunc(catalogDirs, func(a, b promprofiles.DirSpec) int {
		return strings.Compare(filepath.Base(a.Path), filepath.Base(b.Path))
	})
	preflight, err := promprofiles.LoadFromDirs(catalogDirs)
	if err != nil {
		for index, spec := range profileDirs {
			if _, singleErr := promprofiles.LoadFromDirs([]promprofiles.DirSpec{spec}); singleErr != nil {
				return fail(&profileInputError{index: index, code: "profile_load", err: fmt.Errorf("strict profile catalog preflight: %w", singleErr)})
			}
		}
		return fail(fmt.Errorf("strict composed profile catalog preflight: %w", err))
	}
	profiles := make([]promprofiles.Profile, 0, len(profileNames))
	for index, name := range profileNames {
		profile, ok := preflight.Get(name)
		if !ok {
			return fail(&profileInputError{index: index, code: "profile_load", err: fmt.Errorf("strict profile catalog preflight did not load %q", name)})
		}
		if _, err := profile.Template(); err != nil {
			return fail(&profileInputError{index: index, code: "profile_template", err: err})
		}
		if _, err := profile.FallbackType(); err != nil {
			return fail(&profileInputError{index: index, code: "profile_fallback_type", err: err})
		}
		if _, err := profile.Relabeling(); err != nil {
			return fail(&profileInputError{index: index, code: "profile_relabeling", err: err})
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
		dumpPath:     stagedDump,
		dumpRaw:      dumpRaw,
	}, cleanup, nil
}
