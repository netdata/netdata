// SPDX-License-Identifier: GPL-3.0-or-later

// Package profilecatalog is the shared profile-catalog loader used by
// collectors that ship curated per-target "profiles" as YAML files (a profile's
// identity is its file basename). It owns the mechanics common to every such
// collector: walking one or more search directories, resolving stock-vs-user
// override precedence, applying a stock-fatal / user-skip error policy, and
// caching a process-wide catalog.
//
// It is deliberately oblivious to what a profile IS and to how a profile is
// matched to a target. The caller supplies a Decode function that turns file
// bytes into its own profile type P; matching stays in the collector. Decode
// also chooses how deeply to parse: a collector can parse everything eagerly, or
// parse only a lightweight header now and hydrate the heavy part later — lazy
// loading is a property of Decode, not of this package.
package profilecatalog

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
)

// DirSpec is one profile search directory. Stock directories are authoritative:
// a missing stock directory and any invalid stock profile are fatal, while a
// missing user directory is skipped and an invalid user profile is skipped with
// a warning.
type DirSpec struct {
	Path    string
	IsStock bool
}

// FileContext describes the profile file being decoded. IsStock lets a Decode
// function treat stock and user profiles differently (for example, validate a
// user profile eagerly so a broken one is skipped, while deferring stock
// validation).
type FileContext struct {
	BaseName string
	Path     string
	IsStock  bool
}

// Options configures a Load. Only Decode is required.
type Options[P any] struct {
	// Decode turns one profile file's bytes into a P. Returning an error fails a
	// stock profile (fatal) or skips a user profile (logged). Decode decides how
	// deeply to parse (eager or lazy).
	Decode func(ctx FileContext, data []byte) (P, error)
	// NormalizeKey maps a basename to its lookup key. nil means identity; pass
	// strings.ToLower (composed with TrimSpace) for case-insensitive lookup.
	NormalizeKey func(baseName string) string
	// ValidName reports whether a basename may be a profile identity. nil means
	// DefaultValidName.
	ValidName func(baseName string) bool
	// Log receives warnings about skipped user profiles. nil means a package
	// default logger.
	Log *logger.Logger
}

var defaultLog = logger.New().With("component", "go.d/profilecatalog")

// reValidName is the default profile-identity constraint applied to file
// basenames: lowercase, starting with a letter.
var reValidName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// DefaultValidName reports whether name satisfies the default profile-identity
// constraint (`^[a-z][a-z0-9_]*$`). Collectors reuse it to reject names that no
// catalog profile can ever have (config entries, for example).
func DefaultValidName(name string) bool { return reValidName.MatchString(name) }

// loadedEntry is the per-key bookkeeping kept during a walk.
type loadedEntry[P any] struct {
	e    entry[P]
	path string
}

// stockRef records the first stock profile seen for a key, so a second stock
// profile with the same key is rejected even when a user profile shadows both.
type stockRef struct {
	baseName string
	path     string
}

// Load builds a Catalog from the given directories. A profile's identity is its
// file basename. Files that are not `.yaml`/`.yml`, or whose basename starts
// with `_`, are ignored. A user profile overrides a stock profile with the same
// (normalized) key regardless of directory order. Duplicate user profiles are
// skipped with a warning; duplicate stock profiles are fatal. Load never errors
// on an empty result — the caller decides whether an empty catalog is
// acceptable.
func Load[P any](specs []DirSpec, opts Options[P]) (Catalog[P], error) {
	if opts.Decode == nil {
		return Catalog[P]{}, fmt.Errorf("profilecatalog: Options.Decode is required")
	}
	normalize := opts.NormalizeKey
	if normalize == nil {
		normalize = func(s string) string { return s }
	}
	validName := opts.ValidName
	usesDefaultValidName := validName == nil
	if validName == nil {
		validName = DefaultValidName
	}
	log := opts.Log
	if log == nil {
		log = defaultLog
	}

	cat := newCatalog[P](normalize)
	seen := make(map[string]loadedEntry[P])
	stockSeen := make(map[string]stockRef)

	for _, spec := range specs {
		if strings.TrimSpace(spec.Path) == "" {
			continue
		}
		if !DirExists(spec.Path) {
			if spec.IsStock {
				return Catalog[P]{}, fmt.Errorf("stock profiles directory does not exist: %s", spec.Path)
			}
			continue
		}

		err := filepath.WalkDir(spec.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return handleLoadError(spec, log, path, err)
			}
			if d.IsDir() {
				return nil
			}

			name := d.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			if strings.HasPrefix(name, "_") {
				return nil
			}

			baseName := strings.TrimSuffix(name, filepath.Ext(name))
			if !validName(baseName) {
				err := fmt.Errorf("profile %q: invalid basename %q", path, baseName)
				if usesDefaultValidName {
					err = fmt.Errorf("profile %q: basename %q must match %s", path, baseName, reValidName.String())
				}
				return handleLoadError(spec, log, path, err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return handleLoadError(spec, log, path, err)
			}

			prof, err := opts.Decode(FileContext{BaseName: baseName, Path: path, IsStock: spec.IsStock}, data)
			if err != nil {
				return handleLoadError(spec, log, path, err)
			}

			key := normalize(baseName)

			// A second stock profile with the same key is fatal, tracked
			// independently of the effective winner so a user override cannot hide
			// a duplicate stock profile.
			if spec.IsStock {
				if prevStock, dup := stockSeen[key]; dup {
					return fmt.Errorf("duplicate stock profile name %q in %q and %q", prevStock.baseName, prevStock.path, path)
				}
				stockSeen[key] = stockRef{baseName: baseName, path: path}
			}

			prev, exists := seen[key]
			if !exists {
				seen[key] = loadedEntry[P]{
					e:    entry[P]{profile: prof, baseName: baseName, isStock: spec.IsStock},
					path: path,
				}
				cat.orderedKeys = append(cat.orderedKeys, key)
				return nil
			}

			// A duplicate stock profile was already rejected above, so at most one
			// of prev/current is stock here.
			switch {
			case !spec.IsStock && !prev.e.isStock:
				// Both user: keep the first.
				log.Warningf("ignoring duplicate user profile %q in %q; already loaded from %q", baseName, path, prev.path)
			case !spec.IsStock && prev.e.isStock:
				// User profile overrides the stock one.
				seen[key] = loadedEntry[P]{
					e:    entry[P]{profile: prof, baseName: baseName, isStock: false},
					path: path,
				}
			case spec.IsStock && !prev.e.isStock:
				// A user profile already won; stock does not override it.
			}

			return nil
		})
		if err != nil {
			return Catalog[P]{}, err
		}
	}

	for key, le := range seen {
		cat.byKey[key] = le.e
	}
	for key, sr := range stockSeen {
		cat.stockBase[key] = sr.baseName
	}

	return cat, nil
}

// handleLoadError makes a stock-profile error fatal while downgrading a
// user-profile error to a skip-with-warning, so one bad user file cannot stop
// the agent from loading the rest of the catalog.
func handleLoadError(spec DirSpec, log *logger.Logger, path string, err error) error {
	if spec.IsStock {
		return err
	}
	log.Warningf("ignoring invalid user profile %q: %v", path, err)
	return nil
}

// DirExists reports whether dir exists and is a directory.
func DirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}
