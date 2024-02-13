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
	name = filepath.Join(l.dir, name+l.suffix)

	if _, ok := l.locks[name]; ok {
		return true, nil
	}

	locker := flock.New(name)

	ok, err := locker.TryLock()
	if ok {
		l.locks[name] = locker
	} else {
		_ = locker.Close()
	}

	return ok, err
}

func (l *Locker) Unlock(name string) error {
	name = filepath.Join(l.dir, name+l.suffix)

	locker, ok := l.locks[name]
	if !ok {
		return nil
	}

	delete(l.locks, name)

	return locker.Close()
}
