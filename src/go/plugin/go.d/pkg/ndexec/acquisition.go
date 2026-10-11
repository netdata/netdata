// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrAcquisitionBusy       = errors.New("previous acquisition still owns this job")
	ErrSupervisedUnavailable = errors.New("restricted privileged acquisition is unavailable before payload startup")
	ErrSupervisedNotDrained  = errors.New("privileged acquisition cleanup was not verified; job ownership retained")
	ErrSupervisedUnsupported = errors.New("restricted privileged acquisition is unsupported on this platform")
)

// Acquisition captures an immutable process-local job identity. Reconstructing it
// with the same namespace/name shares admission across routes and collector objects.
// An empty name denotes an anonymous owner, distinct from every named owner.
// Callers pass the framework's normalized job name, without further normalization.
// Different names (including a rename) are independent; this is not a host lock.
// The zero value is invalid. Acquisition may be used concurrently.
type Acquisition struct{ key acquisitionKey }
type acquisitionKey struct {
	namespace, name string
	named           bool
}

func NewAcquisition(namespace, name string) *Acquisition {
	return &Acquisition{key: acquisitionKey{namespace: namespace, name: name, named: name != ""}}
}

var acquisitionOwners = struct {
	sync.Mutex
	held map[acquisitionKey]*acquisitionLease
}{held: make(map[acquisitionKey]*acquisitionLease)}

type acquisitionLease struct{ key acquisitionKey }

func (a *Acquisition) acquire() (*acquisitionLease, error) {
	if a == nil || a.key.namespace == "" {
		return nil, errors.New("acquisition namespace is required")
	}
	acquisitionOwners.Lock()
	defer acquisitionOwners.Unlock()
	if acquisitionOwners.held[a.key] != nil {
		return nil, ErrAcquisitionBusy
	}
	lease := &acquisitionLease{key: a.key}
	acquisitionOwners.held[a.key] = lease
	return lease, nil
}

func (l *acquisitionLease) release() {
	acquisitionOwners.Lock()
	defer acquisitionOwners.Unlock()
	if acquisitionOwners.held[l.key] == l {
		delete(acquisitionOwners.held, l.key)
	}
}

// StartUnprivileged preserves StartUnprivilegedProcess's group-signal/leader-reap
// contract. It does not prove whole-tree drain. It shares admission with StartNDSudo.
func (a *Acquisition) StartUnprivileged(ctx context.Context, opts ProcessOptions, path string, args ...string) (*Process, error) {
	lease, err := a.acquire()
	if err != nil {
		return nil, err
	}
	p, err := startProcess(ctx, opts, defaultRunner.ndRunPath, append([]string{path}, args...), func(h processHandle) processHandle {
		return &acquisitionProcess{processHandle: h, lease: lease}
	})
	if err != nil {
		lease.release()
	}
	return p, err
}

type acquisitionProcess struct {
	processHandle
	lease *acquisitionLease
}

func (p *acquisitionProcess) release() error {
	err := p.processHandle.release()
	p.lease.release()
	return err
}

// StartNDSudo starts an explicitly supervised ndsudo command (currently only
// tegrastats), without caller arguments. Startup waits for exec acknowledgement,
// not telemetry. Cancellation closes a private pipe; it never kills the root
// supervisor. Wait may remain unfinished while descendants survive. Missing or
// invalid cleanup evidence retains admission even after Wait returns an error.
// Only ErrSupervisedUnavailable permits a caller to consider unprivileged fallback;
// it proves the helper never started a payload and ownership has been released.
func (a *Acquisition) StartNDSudo(ctx context.Context, opts ProcessOptions, command string) (*Process, error) {
	lease, err := a.acquire()
	if err != nil {
		return nil, err
	}
	if command != "tegrastats" {
		lease.release()
		return nil, errors.New("command does not support restricted supervision")
	}
	return startSupervisedNDSudo(ctx, opts, defaultRunner.ndSudoPath, command, lease)
}
