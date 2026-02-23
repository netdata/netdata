// SPDX-License-Identifier: GPL-3.0-or-later

package filelock

import (
	"path/filepath"

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

	locker := flock.New(filename)

	ok, err := locker.TryLock()
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
