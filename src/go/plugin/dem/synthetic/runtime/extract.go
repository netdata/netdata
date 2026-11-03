// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// saveArchive verifies the entire bounded archive before extraction. The caller
// owns a fresh destination and reserves archive plus expanded bytes beforehand.
func saveArchive(src io.Reader, dst *os.File, a Archive) error {
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(src, a.Bytes))
	if err != nil {
		return fmt.Errorf("copy runtime archive: %w", err)
	}
	var extra [1]byte
	more, extraErr := io.ReadFull(src, extra[:])
	if extraErr != nil && extraErr != io.EOF {
		return fmt.Errorf("read runtime archive ending: %w", extraErr)
	}
	if n != a.Bytes || more != 0 || extraErr != io.EOF || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return errors.New("runtime archive size or SHA256 mismatch")
	}
	return dst.Sync()
}

func extractArchive(src io.Reader, root *os.Root, a Archive) error {
	gz, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gz.Close()
	bounded := &io.LimitedReader{R: gz, N: a.TarBytes + 1}
	reader := tar.NewReader(bounded)
	seen := make(map[string]bool)
	var links []string
	var expanded int64
	entries := 0
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		entries++
		if entries > a.Entries || !localName(name) || reservedName(name) || seen[name] || h.Size < 0 || h.Mode&07000 != 0 {
			return errors.New("invalid, duplicate or excessive runtime archive entry")
		}
		seen[name] = true
		if err := plainParents(root, name); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = root.Mkdir(name, 0700)
			if errors.Is(err, os.ErrExist) {
				info, statErr := root.Lstat(name)
				if statErr == nil && info.IsDir() {
					err = nil
				}
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size > a.ExpandedBytes-expanded {
				return errors.New("runtime archive exceeds declared expanded size")
			}
			expanded += h.Size
			err = extractFile(root, name, reader, h.Size, h.Mode&0111 != 0)
		case tar.TypeSymlink:
			// Only internal links; parents are never allowed to traverse links.
			if path.IsAbs(h.Linkname) || strings.ContainsAny(h.Linkname, "\\\x00\r\n") || !localName(path.Join(path.Dir(name), h.Linkname)) {
				return errors.New("runtime symlink escapes its generation")
			}
			err = root.Symlink(h.Linkname, name)
			links = append(links, name)
		default:
			return errors.New("runtime archive contains unsupported file type")
		}
		if err != nil {
			return err
		}
	}
	// Consume gzip checksum and all declared padding without allowing a bomb.
	if _, err := io.Copy(io.Discard, bounded); err != nil {
		return err
	}
	if bounded.N != 1 || entries != a.Entries || expanded != a.ExpandedBytes {
		return errors.New("runtime archive does not match declared extraction bounds")
	}
	// Lexical checks alone miss escapes through a chain containing "..".
	// Resolve every finished link with the same confinement as extraction.
	// Dangling and cyclic links are not usable runtime dependencies either.
	for _, name := range links {
		if _, err := root.Stat(name); err != nil {
			return fmt.Errorf("runtime symlink target is unavailable inside its generation: %s: %w", name, err)
		}
	}
	return nil
}

func plainParents(root *os.Root, name string) error {
	parent := path.Dir(name)
	if parent == "." {
		return nil
	}
	current := ""
	for _, part := range strings.Split(parent, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			err = root.Mkdir(current, 0700)
		} else if err == nil && !info.IsDir() {
			return errors.New("runtime archive parent is not a plain directory")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func extractFile(root *os.Root, name string, src io.Reader, size int64, executable bool) error {
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.CopyN(f, src, size)
	mode := os.FileMode(0400)
	if executable {
		mode = 0500
	}
	err = errors.Join(copyErr, f.Chmod(mode), f.Sync())
	return errors.Join(err, f.Close())
}
