// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"errors"
	"sync"
)

// Paths is the complete profile search path for one manager. User directories
// are searched in order; duplicate user identities are errors. StockDir is
// required and owns the stock catalogue in its parent directory.
type Paths struct {
	UserDirs []string
	StockDir string
}

// Manager owns the profile epoch shared by all collectors created by one
// plugin registration. The first lease builds an epoch; the final release drops
// it so a later job observes a new on-disk configuration.
type Manager struct {
	mu    sync.Mutex
	paths Paths
	epoch *Epoch
	refs  int
}

// NewManager returns a manager with explicit profile paths.
func NewManager(paths Paths) *Manager {
	paths.UserDirs = append([]string(nil), paths.UserDirs...)
	return &Manager{paths: paths}
}

// Lease is one collector's exact ownership of a shared epoch.
type Lease struct {
	once    sync.Once
	manager *Manager
	epoch   *Epoch
}

// Acquire returns a lease for the current epoch, building it on first use.
func (m *Manager) Acquire() (*Lease, error) {
	if m == nil {
		return nil, errors.New("profile catalog manager is not configured")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.epoch == nil {
		epoch, err := loadEpoch(m.paths)
		if err != nil {
			return nil, err
		}
		m.epoch = epoch
	}
	m.refs++
	return &Lease{manager: m, epoch: m.epoch}, nil
}

// Epoch returns the immutable configuration epoch owned by the lease.
func (l *Lease) Epoch() *Epoch {
	if l == nil {
		return nil
	}
	return l.epoch
}

// Close releases this lease exactly once.
func (l *Lease) Close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.manager != nil {
			l.manager.release(l.epoch)
		}
		l.manager = nil
		l.epoch = nil
	})
}

func (m *Manager) release(epoch *Epoch) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if epoch == nil || m.epoch != epoch || m.refs == 0 {
		return
	}
	m.refs--
	if m.refs == 0 {
		m.epoch = nil
	}
}
