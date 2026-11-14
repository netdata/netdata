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
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	DependencyKind   = "dem-runtime"
	DependencyName   = "browser"
	StateMissing     = "missing"
	StateReady       = "ready"
	StateUnavailable = "unavailable"
)

type Snapshot struct {
	Mode             Mode                       `json:"mode"`
	Running          *synthetic.RuntimeIdentity `json:"running,omitempty"`
	State            string                     `json:"state"`
	SetupError       string                     `json:"setup_error,omitempty"`
	MaintenanceError string                     `json:"maintenance_error,omitempty"`
	Storage          *runtime.Status            `json:"storage,omitempty"`
	Native           *runtime.NativeStatus      `json:"native,omitempty"`
	StorageError     string                     `json:"storage_error,omitempty"`
	RestartRequired  bool                       `json:"restart_required"`
	Update           UpdateStatus               `json:"update"`
}

// Host holds one selected browser runtime for the plugin process. Missing
// installation does not prevent RUM operation; browser jobs wait until an
// installed runtime is selected by a new plugin process.
type Host struct {
	store           *runtime.Store  // managed hosts only; caller-owned
	native          *runtime.Native // native hosts only; caller-owned
	mode            Mode
	consumer        processConsumer
	identity        *synthetic.RuntimeIdentity
	mu              sync.Mutex
	state           string
	setupErr        error
	maintenance     map[string]error
	finishVersion   uint64
	log             *logger.Logger
	started, closed bool
	cancel          context.CancelFunc
	done            chan struct{}
	sweepInterval   time.Duration
	closeOnce       sync.Once
	closeErr        error
	updater         *Updater
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
		state:         StateMissing,
		done:          make(chan struct{}),
		sweepInterval: time.Hour,
	}
}

// NewManaged acquires the store's selected installation and its process
// protection atomically, once. Neither monitor changes nor subsequent
// installations change this selection.
func NewManaged(store *runtime.Store) *Host {
	h := newHost(ModeManaged)
	h.store = store
	if consumer, err := store.AcquireSelected(); err != nil {
		h.fail(err)
	} else {
		h.admit(consumer)
	}
	return h
}

// NewNative acquires the package payload's selection like NewManaged.
func NewNative(native *runtime.Native) *Host {
	h := newHost(ModeNative)
	h.native = native
	if consumer, err := native.AcquireSelected(); err != nil {
		h.fail(err)
	} else {
		h.admit(consumer)
	}
	return h
}

// NewUnavailable reports a setup failure that left the mode without runtime storage.
func NewUnavailable(mode Mode, err error) *Host {
	h := newHost(mode)
	h.fail(err)
	return h
}

func (h *Host) admit(consumer processConsumer) {
	h.consumer = consumer
	h.identity = new(consumer.Manifest().Identity())
	h.state = StateReady
}

func (h *Host) fail(err error) {
	h.state, h.setupErr = StateUnavailable, err
	if errors.Is(err, runtime.ErrUnavailable) {
		h.state = StateMissing
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
	if h.state == StateReady {
		return nil
	}
	if h.setupErr != nil {
		return h.setupErr
	}
	return runtime.ErrUnavailable
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

func (h *Host) Status() Snapshot {
	h.mu.Lock()
	result := Snapshot{Mode: h.mode, State: h.state, SetupError: errorText(h.setupErr)}
	maintenance := h.maintenanceErrorLocked()
	if h.identity != nil {
		identity := *h.identity
		result.Running = &identity
	}
	updater := h.updater
	h.mu.Unlock()
	if updater != nil {
		var updateErr error
		result.Update, updateErr = updater.state()
		maintenance = errors.Join(maintenance, labelled("update", updateErr))
	}
	result.MaintenanceError = errorText(maintenance)
	if h.store != nil {
		status, err := h.store.Inspect()
		result.Storage = &status
		result.StorageError = errorText(err)
		if result.Storage.Selected != nil && result.Storage.SelectionError == "" && result.Storage.InventoryError == "" {
			result.RestartRequired = result.Running == nil || *result.Running != result.Storage.Selected.Identity()
		}
	}
	if h.native != nil {
		status, err := h.native.Inspect()
		result.Native = &status
		result.StorageError = errorText(err)
	}
	return result
}

func (h *Host) BeginUse() (runner.Use, error) {
	if err := h.Ready(); err != nil {
		return nil, err
	}
	use, err := h.consumer.BeginUse()
	if err != nil {
		return nil, err
	}
	return managedUse{use: use, host: h, identity: *h.identity}, nil
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
		h.state, h.setupErr = StateUnavailable, errors.New("browser runtime owner stopped")
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

// Close follows joined jobs, joins the updater and the maintenance worker,
// releases process protection, and leaves the caller-supplied store open.
// Uncertain executions retain their own receipts.
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		h.mu.Lock()
		updater := h.updater
		h.mu.Unlock()
		if updater != nil {
			h.closeErr = updater.Close()
		}
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
			h.closeErr = errors.Join(h.closeErr, h.consumer.Close())
		}
	})
	return h.closeErr
}
