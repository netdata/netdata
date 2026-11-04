// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"syscall"
)

func validateNativeConfig(config NativeConfig) error {
	if goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64" {
		return errors.New("native browser runtime requires Linux amd64 or arm64")
	}
	for _, path := range []string{config.ControlPath, config.PayloadPath} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" || strings.ContainsAny(path, "\x00\r\n") {
			return errors.New("native runtime paths must be clean absolute paths")
		}
	}
	if config.ControlPath == config.PayloadPath || strings.HasPrefix(config.ControlPath, config.PayloadPath+"/") || strings.HasPrefix(config.PayloadPath, config.ControlPath+"/") {
		return errors.New("native payload and proof/control roots must be separate")
	}
	return nil
}

func nativeRootIdentity() error {
	if os.Geteuid() != 0 {
		return errors.New("native package admission administration requires root")
	}
	return nil
}

func nativeServiceIdentity(state nativeControlRecord) error {
	if os.Geteuid() == 0 || os.Geteuid() != state.UID || os.Getegid() != state.GID {
		return errors.New("native runtime must run as its configured service UID/GID")
	}
	return nil
}

func nativeRootOwned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0 && info.Mode().Perm()&0022 == 0 && info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}

// Every ancestor is immutable to the service, including the control/payload
// directory itself. Reject symlink ancestors instead of validating a different
// resolved path from the fixed path used by package hooks and ELF interpreters.
func nativeProtectedPath(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || !nativeRootOwned(info) {
			return fmt.Errorf("native runtime path must be a plain root-owned directory without group/world write access: %s", path)
		}
		if path == "/" {
			return nil
		}
		path = filepath.Dir(path)
	}
}

func nativeProtectedFile(root *os.Root, name string, empty bool) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !nativeRootOwned(info) || (empty && info.Size() != 0) {
		return fmt.Errorf("invalid root-owned native control file: %s", name)
	}
	return nil
}

func nativeProofDirectory(config NativeConfig, state nativeControlRecord) error {
	info, err := os.Lstat(filepath.Join(config.ControlPath, "owners"))
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || int(st.Uid) != state.UID || int(st.Gid) != state.GID {
		return errors.New("native proof directory differs from configured service ownership")
	}
	return nil
}

func createNativeControl(config NativeConfig, uid, gid int) error {
	parent := filepath.Dir(config.ControlPath)
	if err := nativeProtectedPath(parent); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".dem-native-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(temporary)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := writeNewFile(root, nativeGateName, nil, 0644); err != nil {
		return err
	}
	if err := root.Mkdir("owners", 0700); err != nil {
		return err
	}
	if err := os.Chown(filepath.Join(temporary, "owners"), uid, gid); err != nil {
		return err
	}
	initial := nativeControlRecord{Format: 1, Phase: "initializing", PayloadPath: config.PayloadPath, UID: uid, GID: gid}
	stagedConfig := config
	stagedConfig.ControlPath = temporary
	if err := writeNativeControl(stagedConfig, initial); err != nil {
		return err
	}
	if err := errors.Join(syncDirectory(root, "owners"), syncDirectory(root, ".")); err != nil {
		return err
	}
	if err := renameDirectoryExclusive(parent, filepath.Base(temporary), filepath.Base(config.ControlPath)); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	return errors.Join(syncDirectory(parentRoot, "."), parentRoot.Close())
}

func readNativePayload(path string) (Paths, Manifest, error) {
	var empty Manifest
	if err := nativeProtectedPath(path); err != nil {
		return Paths{}, empty, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return Paths{}, empty, err
	}
	defer root.Close()
	var record NativePackage
	if err := nativeProtectedFile(root, "native.json", false); err != nil {
		return Paths{}, empty, err
	}
	if err := readJSON(root, "native.json", &record); err != nil {
		return Paths{}, empty, err
	}
	m := record.Manifest
	if record.Format != 1 || record.Path != path || m.Platform != "linux/"+goruntime.GOARCH {
		return Paths{}, empty, errors.New("native package identity, platform or bound prefix differs")
	}
	if err := m.Validate(); err != nil {
		return Paths{}, empty, err
	}
	// Package authentication authorizes the payload. Verify that its entire
	// reachable tree remains root-controlled and contains no external links.
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok || st.Uid != 0 {
				return errors.New("native runtime symlink is not root-owned")
			}
			_, err := root.Stat(name)
			return err // os.Root rejects dangling, cyclic and escaping targets.
		}
		if (!info.IsDir() && !info.Mode().IsRegular()) || !nativeRootOwned(info) {
			return fmt.Errorf("native payload is not immutable to the service: %s", name)
		}
		return nil
	})
	if err != nil {
		return Paths{}, empty, err
	}
	for _, name := range []string{m.Layout.EntryPoint, m.Layout.Node, m.Layout.Browser, m.Layout.Loader, "chrome", "fonts.conf"} {
		if err := plainExistingParents(root, name); err != nil {
			return Paths{}, empty, err
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			return Paths{}, empty, fmt.Errorf("native payload is missing regular file %s", name)
		}
	}
	for _, name := range []string{m.Layout.Node, m.Layout.Browser, m.Layout.Loader, "chrome"} {
		info, err := root.Lstat(name)
		if err != nil || info.Mode().Perm()&0111 == 0 {
			return Paths{}, empty, fmt.Errorf("native executable is unavailable: %s", name)
		}
	}
	for _, name := range []string{m.Layout.Dependencies, m.Layout.Fonts} {
		if err := plainExistingParents(root, name); err != nil {
			return Paths{}, empty, err
		}
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() {
			return Paths{}, empty, fmt.Errorf("native payload directory is unavailable: %s", name)
		}
	}
	interpreter := []byte(filepath.Join(path, m.Layout.Loader))
	for _, binding := range m.Bindings {
		if err := plainExistingParents(root, binding.Path); err != nil {
			return Paths{}, empty, err
		}
		if err := nativeProtectedFile(root, binding.Path, false); err != nil {
			return Paths{}, empty, err
		}
		if len(interpreter)+1 > len(binding.Expected) {
			return Paths{}, empty, errors.New("native interpreter prefix exceeds binding capacity")
		}
		f, err := root.Open(binding.Path)
		if err != nil {
			return Paths{}, empty, err
		}
		actual := make([]byte, len(binding.Expected))
		_, readErr := f.ReadAt(actual, binding.Offset)
		err = errors.Join(readErr, f.Close())
		if err != nil {
			return Paths{}, empty, err
		}
		bound := make([]byte, len(binding.Expected))
		copy(bound, interpreter)
		if !bytes.Equal(actual, bound) {
			return Paths{}, empty, errors.New("native interpreter is not bound to its package prefix")
		}
	}
	fonts, launcher, err := boundConfiguration(path, m.Layout)
	if err != nil {
		return Paths{}, empty, err
	}
	for name, expected := range map[string][]byte{"fonts.conf": fonts, "chrome": launcher} {
		actual, err := root.ReadFile(name)
		if err != nil {
			return Paths{}, empty, err
		}
		if !bytes.Equal(actual, expected) {
			return Paths{}, empty, fmt.Errorf("native configuration is not bound to its package prefix: %s", name)
		}
	}
	return Paths{EntryPoint: filepath.Join(path, m.Layout.EntryPoint), Node: filepath.Join(path, m.Layout.Node), Dependencies: filepath.Join(path, m.Layout.Dependencies), Browser: filepath.Join(path, "chrome")}, m, nil
}
