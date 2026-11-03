// SPDX-License-Identifier: GPL-3.0-or-later

package filelock

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"

	"github.com/gofrs/flock"
)

func New(dir string) *Locker {
	return &Locker{
		suffix: ".collector.lock",
		dir:    dir,
		locks:  make(map[string]*flock.Flock),
	}
}

type Locker struct {
	suffix string
	dir    string
	locks  map[string]*flock.Flock
}

func (l *Locker) Lock(name string) (bool, error) {
	filename := l.filename(name)

	if _, ok := l.locks[filename]; ok {
		return true, nil
	}

	if err := safefile.CheckRegular(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	flags, err := safefile.NoFollowFlags()
	if err != nil {
		return false, err
	}
	flags |= os.O_CREATE
	// Preserve flock's access-mode requirements and network-filesystem retry behavior.
	switch runtime.GOOS {
	case "aix", "solaris", "illumos":
		flags |= os.O_RDWR
	}
	locker := flock.New(filename, flock.SetFlag(flags))

	ok, err := locker.TryLock()
	if ok {
		// With a held lock Stat inspects the library's descriptor, not the pathname.
		info, statErr := locker.Stat()
		if statErr == nil && !info.Mode().IsRegular() {
			statErr = &os.PathError{
				Op:   "lock",
				Path: filename,
				Err:  safefile.ErrNotRegular,
			}
		}
		if statErr != nil {
			_ = locker.Close()
			return false, statErr
		}
	}
	if ok {
		l.locks[filename] = locker
	} else {
		_ = locker.Close()
	}

	return ok, err
}

func (l *Locker) Unlock(name string) {
	filename := l.filename(name)

	locker, ok := l.locks[filename]
	if !ok {
		return
	}

	delete(l.locks, filename)

	_ = locker.Close()
}

func (l *Locker) UnlockAll() {
	for key, locker := range l.locks {
		delete(l.locks, key)
		_ = locker.Close()
	}
}

func (l *Locker) isLocked(name string) bool {
	_, ok := l.locks[l.filename(name)]
	return ok
}

func (l *Locker) filename(name string) string {
	return filepath.Join(l.dir, name+l.suffix)
}
