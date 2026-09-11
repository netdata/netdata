// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const bundleDiagnosticPath = "06-state/snmp-diagnostics"
const bundleStatusPath = "06-state/snmp-diagnostics-status.txt"

// bundleFS indexes names, not diagnostic payloads. Tar metadata is read during
// inventory so normal-run selection does not need a separate scan per file.
type bundleFS struct {
	file    *os.File
	size    int64
	format  string
	root    string
	entries map[string]bundleEntry
}

type bundleEntry struct {
	info     fs.FileInfo
	zip      *zip.File
	metadata []byte
}

func openBundle(filename, format string) (*bundleFS, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	b := &bundleFS{file: file, size: info.Size(), format: format, entries: make(map[string]bundleEntry)}
	if err := b.index(); err != nil {
		file.Close()
		return nil, err
	}
	return b, nil
}

func (b *bundleFS) Close() error { return b.file.Close() }

func (b *bundleFS) index() error {
	roots := make(map[string]bool)
	add := func(name string, info fs.FileInfo, member *zip.File, reader io.Reader) error {
		name = strings.TrimPrefix(name, "./")
		name = strings.TrimSuffix(name, "/")
		if name == "." || name == "" {
			return nil
		}
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return fmt.Errorf("unsafe bundle member %q", name)
		}
		root, relative, relevant := bundleMember(name)
		if !relevant {
			return nil
		}
		roots[root] = true
		if len(roots) > 1 {
			return errors.New("multiple SNMP support-bundle roots")
		}
		if _, exists := b.entries[name]; exists {
			return fmt.Errorf("duplicate bundle member %q", name)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("bundle evidence is not a regular file: %q", name)
		}
		entry := bundleEntry{info: info, zip: member}
		if member == nil && info.Mode().IsRegular() && (relative == bundleStatusPath || relative == bundleDiagnosticPath+"/normal/runs.json") {
			var err error
			entry.metadata, err = io.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("read bundle metadata %q: %w", name, err)
			}
		}
		b.entries[name] = entry
		return nil
	}
	if b.format == ".zip" {
		archive, err := zip.NewReader(b.file, b.size)
		if err != nil {
			return err
		}
		for _, member := range archive.File {
			if err := add(member.Name, member.FileInfo(), member, nil); err != nil {
				return err
			}
		}
	} else {
		stream, err := b.tarStream()
		if err != nil {
			return err
		}
		defer stream.Close()
		archive := tar.NewReader(stream)
		for {
			header, err := archive.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("read bundle tar: %w", err)
			}
			_, _, relevant := bundleMember(strings.TrimSuffix(strings.TrimPrefix(header.Name, "./"), "/"))
			if relevant && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
				return fmt.Errorf("bundle evidence is not a regular file: %q", header.Name)
			}
			if err := add(header.Name, header.FileInfo(), nil, archive); err != nil {
				return err
			}
		}
		// tar EOF precedes the compression trailer. Validate the outer checksum too.
		if _, err := io.Copy(io.Discard, stream); err != nil {
			return fmt.Errorf("read bundle compression trailer: %w", err)
		}
	}
	if len(roots) == 0 {
		return errors.New("no SNMP support-bundle root found")
	}
	for root := range roots {
		b.root = root
	}
	// ZIP and tar need not contain directory entries. Synthesize them once and
	// reject file/directory collisions rather than depend on archive ordering.
	names := make([]string, 0, len(b.entries))
	for name := range b.entries {
		names = append(names, name)
	}
	for _, name := range names {
		for dir := path.Dir(name); ; dir = path.Dir(dir) {
			if entry, ok := b.entries[dir]; ok {
				if !entry.info.IsDir() {
					return fmt.Errorf("bundle file is also a directory: %q", dir)
				}
			} else {
				b.entries[dir] = bundleEntry{info: bundleDirectory(path.Base(dir))}
			}
			if dir == "." {
				break
			}
		}
	}
	return nil
}

// Producers use one wrapper directory. Also accept a bundle rooted directly at
// MANIFEST.json/06-state, but never recursively search arbitrary descendants.
func bundleMember(name string) (root, relative string, relevant bool) {
	matches := func(s string) bool {
		return s == "MANIFEST.json" || s == bundleStatusPath || s == bundleDiagnosticPath || strings.HasPrefix(s, bundleDiagnosticPath+"/")
	}
	if matches(name) {
		return ".", name, true
	}
	wrapper, rest, ok := strings.Cut(name, "/")
	if ok && matches(rest) {
		return wrapper, rest, true
	}
	return "", "", false
}

func (b *bundleFS) tarStream() (io.ReadCloser, error) {
	reader := io.NewSectionReader(b.file, 0, b.size)
	if b.format == ".tar.gz" {
		return gzip.NewReader(reader)
	}
	decoder, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	return decoder.IOReadCloser(), nil
}

func (b *bundleFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := b.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if entry.info.IsDir() {
		return nil, fmt.Errorf("cannot open bundle directory %q as evidence", name)
	}
	var reader io.ReadCloser
	var err error
	_, relative, _ := bundleMember(name)
	if entry.zip != nil {
		// ZIP can isolate a damaged metadata member without preventing access
		// to intact topology/lifecycle documents elsewhere in the container.
		reader, err = entry.zip.Open()
	} else if relative == bundleStatusPath || relative == bundleDiagnosticPath+"/normal/runs.json" {
		reader = io.NopCloser(bytes.NewReader(entry.metadata))
	} else {
		reader, err = b.openTarMember(name)
	}
	if err != nil {
		return nil, err
	}
	return &bundleFile{ReadCloser: reader, info: entry.info}, nil
}

func (b *bundleFS) openTarMember(name string) (io.ReadCloser, error) {
	stream, err := b.tarStream()
	if err != nil {
		return nil, err
	}
	archive := tar.NewReader(stream)
	for {
		header, err := archive.Next()
		if err != nil {
			stream.Close()
			return nil, fmt.Errorf("open bundle member %q: %w", name, err)
		}
		if strings.TrimPrefix(header.Name, "./") == name {
			return &memberReadCloser{Reader: archive, Closer: stream}, nil
		}
	}
}

func (b *bundleFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := b.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	if !entry.info.IsDir() {
		return nil, fmt.Errorf("bundle member %q is not a directory", name)
	}
	var entries []fs.DirEntry
	for filename, entry := range b.entries {
		if filename != name && path.Dir(filename) == name {
			entries = append(entries, fs.FileInfoToDirEntry(entry.info))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (b *bundleFS) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	entry, ok := b.entries[name]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return entry.info, nil
}

type memberReadCloser struct {
	io.Reader
	io.Closer
}
type bundleFile struct {
	io.ReadCloser
	info fs.FileInfo
}

func (f *bundleFile) Stat() (fs.FileInfo, error) { return f.info, nil }

type bundleDirectory string

func (d bundleDirectory) Name() string       { return string(d) }
func (d bundleDirectory) Size() int64        { return 0 }
func (d bundleDirectory) Mode() fs.FileMode  { return fs.ModeDir | 0700 }
func (d bundleDirectory) ModTime() time.Time { return time.Time{} }
func (d bundleDirectory) IsDir() bool        { return true }
func (d bundleDirectory) Sys() any           { return nil }
