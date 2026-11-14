// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func randomID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ensureLock(root *os.Root, name string) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		err = errors.Join(f.Sync(), f.Close())
	} else if errors.Is(err, os.ErrExist) {
		info, statErr := root.Lstat(name)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
			return errors.New("invalid runtime lock file")
		}
		return nil
	}
	return err
}

func syncDirectory(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func syncTree(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return syncDirectory(root, name)
		}
		return nil
	})
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

type metadataError struct {
	name string
	err  error
}

func (e *metadataError) Error() string { return fmt.Sprintf("%s: %v", e.name, e.err) }

func (e *metadataError) Unwrap() error { return e.err }

func isMetadataDamage(err error) bool {
	var damage *metadataError
	return errors.As(err, &damage)
}

func damagedMetadata(name string, err error) error {
	return &metadataError{
		name: name,
		err:  err,
	}
}

func metadataDecodeError(name string, err error) error {
	// os.File.Read wraps operational errors in PathError. They must not become
	// permission to replace data, including an error from the trailing-value read.
	var operational *os.PathError
	if errors.As(err, &operational) {
		return err
	}
	return damagedMetadata(name, err)
}

func readJSON(root *os.Root, name string, value any) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("runtime metadata is not a regular file")
	}
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return metadataDecodeError(name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return metadataDecodeError(name, err)
		}
		return damagedMetadata(name, errors.New("trailing runtime metadata"))
	}
	return nil
}

func saveJSON(root *os.Root, name string, value any) error {
	if err := publishJSON(root, name, value); err != nil {
		return err
	}
	return syncDirectory(root, filepath.Dir(name))
}

func publishJSON(root *os.Root, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	temporary := name + "." + id
	if err := writeNewFile(root, temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := root.Rename(temporary, name); err != nil {
		return err
	}
	return nil
}

// retireToTrash is shared durability machinery, not retirement policy. Callers
// authorize the namespace and complete payload-specific publication barriers.
func retireToTrash(root *os.Root, name string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	if err := root.Rename(name, filepath.Join("trash", id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// Re-sync even when a successor already moved the record or fsync failed.
	return errors.Join(syncDirectory(root, filepath.Dir(name)), syncDirectory(root, "trash"))
}
