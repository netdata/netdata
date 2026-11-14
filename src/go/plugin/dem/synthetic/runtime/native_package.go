// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// PrepareNativePackage assembles a package payload without executing it or
// touching installed runtime state. The builder owns the trusted manifest and
// exclusive access to output, which must be a new or empty staging directory.
// Package recipes must assign root ownership before distribution. A failed stage
// is retained for inspection; only a successful call authorizes packaging it.
func PrepareNativePackage(ctx context.Context, m Manifest, archivePath, output string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	config := DefaultNativeConfig()
	parent, root, err := openNativeStaging(output, config)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := extractNativeArchive(ctx, root, parent, archivePath, m.Archive); err != nil {
		return err
	}
	if err := bindPrefix(root, config.PayloadPath, m); err != nil {
		return err
	}
	if err := normalizeNativeModes(root); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return publishNativeMetadata(root, parent, output, NativePackage{
		Format:   1,
		Path:     config.PayloadPath,
		Manifest: m,
	})
}

// openNativeStaging returns the resolved parent and a root for a new or empty
// plain staging directory separate from the installed payload and control paths.
func openNativeStaging(output string, config NativeConfig) (string, *os.Root, error) {
	if !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return "", nil, errors.New("native package output must be a clean absolute staging path")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return "", nil, err
	}
	resolved := filepath.Join(parent, filepath.Base(output))
	for _, installed := range []string{config.PayloadPath, config.ControlPath} {
		if resolved == installed || strings.HasPrefix(resolved, installed+string(os.PathSeparator)) || strings.HasPrefix(installed, resolved+string(os.PathSeparator)) {
			return "", nil, errors.New("native package staging must be separate from installed payload and control paths")
		}
	}
	if err := os.Mkdir(output, 0700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", nil, err
		}
		info, err := os.Lstat(output)
		if err != nil {
			return "", nil, err
		}
		if !info.IsDir() {
			return "", nil, errors.New("native package output must be a plain directory")
		}
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		return "", nil, err
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err == nil && len(entries) != 0 {
		err = errors.New("native package output must be empty; existing files are never replaced")
	}
	if err != nil {
		root.Close()
		return "", nil, err
	}
	return parent, root, nil
}

// extractNativeArchive verifies the archive in a spool beside the staging
// directory before extracting it, so no archive bytes enter the payload tree.
func extractNativeArchive(ctx context.Context, root *os.Root, parent, archivePath string, a Archive) error {
	source, err := safefile.Open(archivePath)
	if err != nil {
		return err
	}
	defer source.Close()
	spool, err := os.CreateTemp(parent, ".dem-native-archive-*")
	if err != nil {
		return err
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	if err := saveArchive(contextReader{
		ctx:    ctx,
		source: source,
	}, spool, a); err != nil {
		return err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extractArchive(contextReader{
		ctx:    ctx,
		source: spool,
	}, root, a)
}

// normalizeNativeModes makes the root-owned package readable by the service UID:
// directories and executables become 0755, other files 0644.
func normalizeNativeModes(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		mode := os.FileMode(0644)
		if info.IsDir() || info.Mode().Perm()&0111 != 0 {
			mode = 0755
		}
		return root.Chmod(name, mode)
	})
}

// publishNativeMetadata completes the stage: native.json appears only after the
// payload tree is synced. A sibling temporary file keeps generated metadata
// outside archive names.
func publishNativeMetadata(root *os.Root, parent, output string, record NativePackage) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	metadata, err := os.CreateTemp(parent, ".dem-native-metadata-*")
	if err != nil {
		return err
	}
	defer os.Remove(metadata.Name())
	if _, err := metadata.Write(data); err != nil {
		metadata.Close()
		return err
	}
	if err := errors.Join(metadata.Chmod(0644), metadata.Sync(), metadata.Close()); err != nil {
		return err
	}
	if err := syncTree(root); err != nil {
		return err
	}
	if err := os.Rename(metadata.Name(), filepath.Join(output, "native.json")); err != nil {
		return fmt.Errorf("complete native package metadata: %w", err)
	}
	return syncDirectory(root, ".")
}
