// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type UpdateStatus struct {
	Checking      bool  `json:"checking"`
	LastCheckUS   int64 `json:"last_check_us,omitempty"`
	LastSuccessUS int64 `json:"last_success_us,omitempty"`
}

type Qualifier interface {
	Qualify(context.Context, runner.Source) (*runner.Qualification, error)
}

// Updater is a process service, independent of monitor configuration and storage
// maintenance. It prepares selections; it never restarts or switches consumers.
type Updater struct {
	store     *runtime.Store
	qualifier Qualifier
	load      func(context.Context) (runtime.VerifiedRelease, error)
	delay     func(bool) time.Duration
	log       *logger.Logger
	mu        sync.Mutex
	status    UpdateStatus
	err       error // last completed check; cleared only by a later successful check
	started   bool
	closed    bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewUpdater attaches automatic release preparation to a managed host, at most
// once. Host.Status reports its progress and Host.Close joins it before
// releasing process protection. Hosts without a managed store return nil.
func (h *Host) NewUpdater(qualifier Qualifier, load func(context.Context) (runtime.VerifiedRelease, error)) *Updater {
	if h.store == nil {
		return nil
	}
	u := &Updater{
		store:     h.store,
		qualifier: qualifier,
		load:      load,
		log:       h.log,
		done:      make(chan struct{}),
		delay: func(initial bool) time.Duration {
			if initial {
				return time.Duration(rand.Int64N(int64(5 * time.Minute)))
			}
			return 23*time.Hour + time.Duration(rand.Int64N(int64(2*time.Hour)))
		},
	}
	h.mu.Lock()
	h.updater = u
	h.mu.Unlock()
	return u
}

func (u *Updater) Run(ctx context.Context) {
	u.mu.Lock()
	if u.started || u.closed {
		u.mu.Unlock()
		return
	}
	u.started = true
	ctx, u.cancel = context.WithCancel(ctx)
	u.mu.Unlock()
	defer close(u.done)
	timer := time.NewTimer(u.delay(true))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := u.check(ctx); errors.Is(err, ndexec.ErrTreeNotDrained) {
				// The shared execution owner already notified the fatal watcher.
				return
			}
			timer.Reset(u.delay(false))
		}
	}
}

func (u *Updater) check(ctx context.Context) error {
	u.mu.Lock()
	u.status.Checking = true
	u.mu.Unlock()
	discovered := false
	err := u.prepare(ctx, func(ctx context.Context) (runtime.VerifiedRelease, error) {
		discovered = true
		u.mu.Lock()
		u.status.LastCheckUS = time.Now().UnixMicro()
		u.mu.Unlock()
		return u.load(ctx)
	})
	u.mu.Lock()
	u.status.Checking = false
	if discovered && err == nil {
		u.status.LastSuccessUS = time.Now().UnixMicro()
	}
	var change healthChange
	if ctx.Err() == nil {
		change = trackHealth("update", u.err, err)
		u.err = err
	}
	u.mu.Unlock()
	change.log(u.log)
	return err
}

// prepare runs one complete managed transaction. Qualification uses the
// process's existing execution owner. Without consent or a newer release no
// candidate is begun.
func (u *Updater) prepare(ctx context.Context, load func(context.Context) (runtime.VerifiedRelease, error)) (result error) {
	options := runtime.ReleaseInstallOptions{Operation: runtime.AutomaticReleaseUpdate}
	candidate, err := u.store.BeginPublishedInstall(ctx, options, load)
	if err != nil || candidate == nil {
		return err
	}
	defer func() {
		if !errors.Is(result, ndexec.ErrTreeNotDrained) {
			result = errors.Join(result, candidate.Close())
		}
	}()
	if err := candidate.Download(ctx); err != nil {
		return err
	}
	if _, err := u.qualifier.Qualify(ctx, Borrow(candidate)); err != nil {
		return err
	}
	return candidate.Select()
}

// state reports the progress and last failure that Host.Status publishes.
func (u *Updater) state() (UpdateStatus, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status, u.err
}

// Close cancels acquisition and joins qualification before its caller closes
// runtime stores. Fail-stop execution proof is never converted into cleanup.
func (u *Updater) Close() error {
	u.mu.Lock()
	u.closed = true
	if u.cancel != nil {
		u.cancel()
	}
	started := u.started
	u.mu.Unlock()
	if started {
		<-u.done
	}
	return nil
}
