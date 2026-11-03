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
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	if !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return errors.New("native package output must be a clean absolute staging path")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return err
	}
	resolved := filepath.Join(parent, filepath.Base(output))
	config := DefaultNativeConfig()
	for _, installed := range []string{config.PayloadPath, config.ControlPath} {
		if resolved == installed || strings.HasPrefix(resolved, installed+string(os.PathSeparator)) || strings.HasPrefix(installed, resolved+string(os.PathSeparator)) {
			return errors.New("native package staging must be separate from installed payload and control paths")
		}
	}
	if err := os.Mkdir(output, 0700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(output)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("native package output must be a plain directory")
		}
	}
	root, err := os.OpenRoot(output)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("native package output must be empty; existing files are never replaced")
	}
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
	}, spool, m.Archive); err != nil {
		return err
	}
	if _, err := spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := extractArchive(contextReader{
		ctx:    ctx,
		source: spool,
	}, root, m.Archive); err != nil {
		return err
	}
	// Native admission requires plain parents for every declared layout path,
	// including the loader and data directories that have no interpreter slot.
	for _, name := range []string{m.Layout.EntryPoint, m.Layout.Node, m.Layout.Browser, m.Layout.Loader, m.Layout.Dependencies, m.Layout.Fonts} {
		if err := plainExistingParents(root, name); err != nil {
			return err
		}
	}
	if err := bindPrefix(root, config.PayloadPath, m); err != nil {
		return err
	}
	if err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
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
	}); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	data, err := json.Marshal(NativePackage{
		Format:   1,
		Path:     config.PayloadPath,
		Manifest: m,
	})
	if err != nil {
		return err
	}
	// A sibling temporary file keeps generated metadata outside archive names.
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
