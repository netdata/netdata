// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func validCompletionID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func openCompletionDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		f.Close()
		return nil, err
	}
	if st.Uid != uint32(os.Geteuid()) || st.Mode&0777 != 0700 {
		f.Close()
		return nil, fmt.Errorf("completion directory must be private to the execution account")
	}
	return f, nil
}

func completionFile(dir *os.File, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, flags|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		f.Close()
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Mode&0777 != 0600 || st.Nlink != 1 {
		f.Close()
		return nil, fmt.Errorf("invalid completion record file %s", name)
	}
	return f, nil
}

// CreateTreeCompletion creates and durably publishes a fresh private directory.
// Its parent must already exist. On error, partial records are retained for the
// caller to account/clean; no command has been started. This does not persist the
// caller's resource binding, which MUST be durable before execution begins.
func CreateTreeCompletion(path string) (*TreeCompletion, error) {
	path = filepath.Clean(path)
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(nonce[:])
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	dir, err := openCompletionDirectory(path)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			dir.Close()
		}
	}()
	for _, record := range []struct{ name, data string }{
		{"publication.lock", ""},
		{"pending", "NDTREE1 pending " + id + "\n"},
	} {
		f, err := completionFile(dir, record.name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
		if err != nil {
			return nil, err
		}
		_, writeErr := io.WriteString(f, record.data)
		err = errors.Join(writeErr, f.Sync(), f.Close())
		if err != nil {
			return nil, err
		}
	}
	if err := dir.Sync(); err != nil {
		return nil, err
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if err := errors.Join(parent.Sync(), parent.Close()); err != nil {
		return nil, err
	}
	success = true
	return &TreeCompletion{directory: dir, id: id}, nil
}

func matchingCompletionRecord(dir *os.File, name, expected string) error {
	f, err := completionFile(dir, name, unix.O_RDONLY)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(len(expected)+1)))
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if !bytes.Equal(data, []byte(expected)) {
		return fmt.Errorf("invalid %s completion record", name)
	}
	return nil
}

// ReadTreeCompletion reports persistent verified quiescence for this exact ID.
// false means pending/publishing. Any error is uncertainty, never permission to
// release resources. A true result also proves the supervisor finished record
// I/O, so the owner may remove the record. Never infer completion from a free
// lock, PID disappearance or record age. Only the trusted supervisor writes it.
func ReadTreeCompletion(path, id string) (bool, error) {
	if !validCompletionID(id) {
		return false, os.ErrInvalid
	}
	dir, err := openCompletionDirectory(path)
	if err != nil {
		return false, err
	}
	defer dir.Close()
	lock, err := completionFile(dir, "publication.lock", unix.O_RDWR)
	if err != nil {
		return false, err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}
	if err := matchingCompletionRecord(dir, "pending", "NDTREE1 pending "+id+"\n"); err != nil {
		return false, err
	}
	err = matchingCompletionRecord(dir, "complete", "NDTREE1 drained "+id+"\n")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
