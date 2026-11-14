// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// bindPrefix operates only on unpublished, digest-verified staging files. The
// interpreter names the FINAL generation, never the staging directory.
func bindPrefix(root *os.Root, final string, m Manifest) error {
	if !filepath.IsAbs(final) || filepath.Clean(final) != final || strings.ContainsAny(final, "\x00\r\n") || !utf8.ValidString(final) {
		return errors.New("runtime prefix must be a clean absolute UTF-8 path")
	}
	// os.Root can traverse paths longer than Linux's absolute pathname limit.
	// Node and the launcher later open absolute names, so every delivered file
	// must fit, not just the shorter PT_INTERP loader name.
	if err := fs.WalkDir(root.FS(), ".", func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if len(filepath.Join(final, name)) >= 4096 {
			return errors.New("runtime payload exceeds Linux absolute path limit")
		}
		return nil
	}); err != nil {
		return err
	}
	for _, name := range []string{"chrome", "fonts.conf"} {
		if len(filepath.Join(final, name)) >= 4096 {
			return errors.New("runtime payload exceeds Linux absolute path limit")
		}
	}
	interpreter := []byte(filepath.Join(final, filepath.FromSlash(m.Layout.Loader)))
	for _, binding := range m.Bindings {
		if len(interpreter)+1 > len(binding.Expected) {
			return errors.New("runtime prefix exceeds authenticated interpreter capacity")
		}
		if err := bindInterpreter(root, binding, interpreter); err != nil {
			return err
		}
	}
	fonts, launcher, err := boundConfiguration(final, m.Layout)
	if err != nil {
		return err
	}
	if err := writeNewFile(root, "fonts.conf", fonts, 0400); err != nil {
		return err
	}
	if err := writeNewFile(root, "chrome", launcher, 0500); err != nil {
		return err
	}
	// Staging is owned and, for managed generations, later executed by this account.
	return verifyBoundPayload(root, m, ownerExecute)
}

// payloadLayoutError records a directly observed structural violation.
type payloadLayoutError string

func (e payloadLayoutError) Error() string { return string(e) }

// Execute permission classes for verifyBoundPayload. Linux grants execution
// through the owner bits when the caller owns the file, the group bits when it
// is in the file's group, and the other bits otherwise. Managed generations are
// owned and executed by the service account. Native payloads are root-owned and
// executed by the non-root service UID, so their owner bit never authorizes it;
// qualification, which runs the payload as that UID, remains the full proof.
const (
	ownerExecute    os.FileMode = 0100
	nonOwnerExecute os.FileMode = 0011
)

// verifyBoundPayload checks the fixed entry paths of a bound payload without
// walking or executing it: every path has plain directory parents, the declared
// executables and the generated launcher are regular files with the execute bit
// of the class that runs them, the entrypoint and generated font configuration
// are regular files, and the data paths are directories.
func verifyBoundPayload(root *os.Root, m Manifest, execute os.FileMode) error {
	for _, name := range []string{m.Layout.Node, m.Layout.Browser, m.Layout.Loader, "chrome"} {
		info, err := lstatPlain(root, name)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&execute == 0 {
			return payloadLayoutError(fmt.Sprintf("runtime executable is missing or not a plain executable: %s", name))
		}
	}
	for _, name := range []string{m.Layout.EntryPoint, "fonts.conf"} {
		info, err := lstatPlain(root, name)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return payloadLayoutError(fmt.Sprintf("runtime file is not a plain regular file: %s", name))
		}
	}
	for _, name := range []string{m.Layout.Dependencies, m.Layout.Fonts} {
		info, err := lstatPlain(root, name)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return payloadLayoutError(fmt.Sprintf("runtime directory is missing: %s", name))
		}
	}
	return nil
}

func lstatPlain(root *os.Root, name string) (os.FileInfo, error) {
	if err := plainExistingParents(root, name); err != nil {
		return nil, err
	}
	return root.Lstat(name)
}

// boundPaths names the executable entry points of a payload bound to prefix.
func boundPaths(prefix string, layout Layout) Paths {
	return Paths{
		EntryPoint:   filepath.Join(prefix, layout.EntryPoint),
		Node:         filepath.Join(prefix, layout.Node),
		Dependencies: filepath.Join(prefix, layout.Dependencies),
		Browser:      filepath.Join(prefix, "chrome"),
	}
}

func boundConfiguration(final string, layout Layout) ([]byte, []byte, error) {
	var fonts bytes.Buffer
	fonts.WriteString("<?xml version=\"1.0\"?><!DOCTYPE fontconfig SYSTEM \"urn:fontconfig:fonts.dtd\"><fontconfig><dir>")
	if err := xml.EscapeText(&fonts, []byte(filepath.Join(final, layout.Fonts))); err != nil {
		return nil, nil, err
	}
	fonts.WriteString("</dir><cachedir prefix=\"xdg\">fontconfig</cachedir><config><rescan><int>0</int></rescan></config></fontconfig>\n")
	// /bin/sh is an installation prerequisite, not a host Node/libc dependency.
	launcher := "#!/bin/sh\nexport FONTCONFIG_FILE=" + shellQuote(filepath.Join(final, "fonts.conf")) +
		"\nexec " + shellQuote(filepath.Join(final, layout.Browser)) + " \"$@\"\n"
	return fonts.Bytes(), []byte(launcher), nil
}

func bindInterpreter(root *os.Root, binding Binding, interpreter []byte) error {
	if err := plainExistingParents(root, binding.Path); err != nil {
		return err
	}
	info, err := root.Lstat(binding.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || binding.Offset > info.Size() || int64(len(binding.Expected)) > info.Size()-binding.Offset {
		return errors.New("interpreter binding lies outside a regular file")
	}
	if err := root.Chmod(binding.Path, 0600); err != nil {
		return err
	}
	f, err := root.OpenFile(binding.Path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	actual := make([]byte, len(binding.Expected))
	_, err = f.ReadAt(actual, binding.Offset)
	if err != nil || !bytes.Equal(actual, binding.Expected) {
		f.Close()
		return errors.New("interpreter slot does not match authenticated bytes")
	}
	bound := make([]byte, len(actual))
	copy(bound, interpreter)
	_, err = f.WriteAt(bound, binding.Offset)
	err = errors.Join(err, f.Chmod(info.Mode().Perm()), f.Sync())
	return errors.Join(err, f.Close())
}

func plainExistingParents(root *os.Root, name string) error {
	var parents []string
	for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
		parents = append(parents, parent)
	}
	// Inspect outer ancestors before traversing them, so invalid symlinks are
	// observed as structural violations rather than failed child lookups.
	for i := len(parents) - 1; i >= 0; i-- {
		info, err := root.Lstat(parents[i])
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return payloadLayoutError("runtime path contains a non-directory parent")
		}
	}
	return nil
}

func writeNewFile(root *os.Root, name string, contents []byte, mode os.FileMode) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, bytes.NewReader(contents))
	err = errors.Join(err, f.Chmod(mode), f.Sync())
	return errors.Join(err, f.Close())
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
