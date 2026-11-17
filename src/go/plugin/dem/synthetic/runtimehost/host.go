// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtimehost opens the browser runtime selected for the plugin process
// and adapts it to the runner and the job framework.
package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	DependencyKind = "dem-runtime"
	DependencyName = "browser"
)

// Host holds one selected browser runtime for the plugin process. Missing
// installation does not prevent RUM operation; browser jobs wait until an
// installed runtime is selected by a new plugin process.
type Host struct {
	store           *browserruntime.Store  // managed hosts only; caller-owned
	native          *browserruntime.Native // native hosts only; caller-owned
	mode            Mode
	consumer        processConsumer
	identity        *synthetic.RuntimeIdentity
	mu              sync.Mutex
	setupErr        error // why the host is not ready; nil once admitted
	maintenance     map[string]error
	finishVersion   uint64
	log             *logger.Logger
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	sweepInterval   time.Duration
	closeOnce       sync.Once
	closeErr        error
}

type processConsumer interface {
	Consumer
	Close() error
}

var _ runner.Source = (*Host)(nil)

func newHost(mode Mode) *Host {
	return &Host{
		mode:          mode,
		maintenance:   make(map[string]error),
		log:           logger.New(),
		done:          make(chan struct{}),
		sweepInterval: time.Hour,
	}
}

// newManaged acquires the store's selected installation and its process
// protection atomically, once. Neither monitor changes nor subsequent
// installations change this selection.
func newManaged(store *browserruntime.Store) *Host {
	h := newHost(ModeManaged)
	h.store = store
	if consumer, err := store.AcquireSelected(); err != nil {
		h.fail(err)
	} else {
		h.admit(consumer)
	}
	return h
}

// newNative acquires the package payload's selection like newManaged.
func newNative(native *browserruntime.Native) *Host {
	h := newHost(ModeNative)
	h.native = native
	if consumer, err := native.AcquireSelected(); err != nil {
		h.fail(err)
	} else {
		h.admit(consumer)
	}
	return h
}

// newUnavailable reports a setup failure that left the mode without runtime storage.
func newUnavailable(mode Mode, err error) *Host {
	h := newHost(mode)
	h.fail(err)
	return h
}

func (h *Host) admit(consumer processConsumer) {
	h.consumer = consumer
	h.identity = new(consumer.Manifest().Identity())
}

func (h *Host) fail(err error) {
	h.setupErr = err
	if errors.Is(err, browserruntime.ErrUnavailable) {
		h.setupErr = fmt.Errorf("install a browser runtime and restart dem.plugin: %w", err)
		if h.mode == ModeNative {
			h.setupErr = fmt.Errorf("install the optional netdata-dem-runtime package and restart dem.plugin: %w", err)
		}
	}
}

func (h *Host) Ready() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return errors.New("browser runtime owner is closed")
	}
	if h.setupErr != nil {
		return h.setupErr
	}
	if h.consumer == nil {
		return browserruntime.ErrUnavailable
	}
	return nil
}

func (h *Host) Dependencies(map[string]any) ([]collectorapi.ActivationDependency, error) {
	return []collectorapi.ActivationDependency{{Kind: DependencyKind, Name: DependencyName, Ready: h.Ready}}, nil
}

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func (h *Host) BeginUse() (runner.Use, error) {
	if err := h.Ready(); err != nil {
		return nil, err
	}
	use, err := h.consumer.BeginUse()
	if err != nil {
		return nil, err
	}
	return runtimeUse{
		use:      use,
		host:     h,
		identity: *h.identity,
	}, nil
}

// Run maintains storage independently of monitoring configuration. It never
// downloads, prepares, or changes the process's selected runtime.
func (h *Host) Run(ctx context.Context) {
	h.mu.Lock()
	if h.started || h.closed {
		h.mu.Unlock()
		return
	}
	h.started = true
	ctx, h.cancel = context.WithCancel(ctx)
	h.mu.Unlock()
	defer close(h.done)
	defer func() {
		h.mu.Lock()
		h.setupErr = errors.New("browser runtime owner stopped")
		h.mu.Unlock()
	}()
	sweep := time.NewTicker(h.sweepInterval)
	defer sweep.Stop()
	h.sweep()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			h.sweep()
		}
	}
}

func (h *Host) sweep() {
	if h.store == nil && h.native == nil {
		return
	}
	h.mu.Lock()
	version := h.finishVersion
	h.mu.Unlock()
	var err error
	if h.native != nil {
		err = h.native.Sweep()
	} else {
		err = h.store.Sweep()
	}
	h.setMaintenance("cleanup", err)
	h.mu.Lock()
	// A newer Finish failure must not be cleared by an older sweep.
	var change healthChange
	if err == nil && version == h.finishVersion {
		change = h.updateMaintenanceLocked("execution completion", nil)
	}
	h.mu.Unlock()
	change.log(h.log)
}

// Close follows joined jobs, joins the maintenance worker,
// releases process protection, and leaves the caller-supplied store open.
// Uncertain executions retain their own receipts.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.closed = true
		if h.cancel != nil {
			h.cancel()
		}
		started := h.started
		h.mu.Unlock()
		if started {
			<-h.done
		}
		if h.consumer != nil {
			h.closeErr = h.consumer.Close()
		}
	})
	return h.closeErr
}
