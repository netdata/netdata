// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

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

// PrepareRelease owns one complete managed transaction. Qualification must use
// the process's existing execution owner when called inside a running plugin.
// A nil result with no error means no candidate needed preparation.
func PrepareRelease(ctx context.Context, store *runtime.Store, options runtime.ReleaseInstallOptions,
	load func(context.Context) (runtime.Release, error), archive string, qualifier Qualifier) (_ *runner.Qualification, result error) {
	if qualifier == nil {
		return nil, errors.New("runtime qualifier is unavailable")
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil || candidate == nil {
		return nil, err
	}
	defer func() {
		if !errors.Is(result, ndexec.ErrTreeNotDrained) {
			result = errors.Join(result, candidate.Close())
		}
	}()
	if archive == "" {
		err = candidate.Download(ctx)
	} else {
		err = candidate.Import(ctx, archive)
	}
	if err != nil {
		return nil, err
	}
	qualification, err := qualifier.Qualify(ctx, Borrow(candidate))
	if err != nil {
		return qualification, err
	}
	return qualification, candidate.Select()
}

// Updater is a process service, independent of monitor configuration and storage
// maintenance. It prepares selections; it never restarts or switches consumers.
type Updater struct {
	host      *Host
	qualifier Qualifier
	load      func(context.Context) (runtime.Release, error)
	delay     func(bool) time.Duration
	mu        sync.Mutex
	started   bool
	closed    bool
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewUpdater(host *Host, qualifier Qualifier, load func(context.Context) (runtime.Release, error)) *Updater {
	return &Updater{host: host, qualifier: qualifier, load: load, done: make(chan struct{}), delay: func(initial bool) time.Duration {
		if initial {
			return time.Duration(rand.Int64N(int64(5 * time.Minute)))
		}
		return 23*time.Hour + time.Duration(rand.Int64N(int64(2*time.Hour)))
	}}
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
	if u.host == nil || u.host.store == nil {
		return nil
	}
	u.host.mu.Lock()
	u.host.update.Checking = true
	u.host.mu.Unlock()
	discovered := false
	_, err := PrepareRelease(ctx, u.host.store, runtime.ReleaseInstallOptions{Operation: runtime.AutomaticReleaseUpdate}, func(ctx context.Context) (runtime.Release, error) {
		discovered = true
		u.host.mu.Lock()
		u.host.update.LastCheckUS = time.Now().UnixMicro()
		u.host.mu.Unlock()
		if u.load == nil {
			return runtime.Release{}, errors.New("runtime release discovery is not configured")
		}
		return u.load(ctx)
	}, "", u.qualifier)
	u.host.mu.Lock()
	u.host.update.Checking = false
	if discovered && err == nil {
		u.host.update.LastSuccessUS = time.Now().UnixMicro()
	}
	u.host.mu.Unlock()
	if ctx.Err() == nil {
		u.host.setMaintenance("update", err)
	}
	return err
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
