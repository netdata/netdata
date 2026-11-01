// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtimehost holds one selected browser runtime for the plugin process.
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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const (
	DependencyKind   = "dem-runtime"
	DependencyName   = "browser"
	StateMissing     = "missing"
	StateReady       = "ready"
	StateUnavailable = "unavailable"
)

// Config supplies a caller-owned store. Missing installation does not prevent
// RUM operation; browser jobs wait until an installed runtime is selected by a
// new plugin process.
type Config struct {
	Store       *runtime.Store
	Unavailable error
}

type Snapshot struct {
	Running          *synthetic.RuntimeIdentity `json:"running,omitempty"`
	State            string                     `json:"state"`
	SetupError       string                     `json:"setup_error,omitempty"`
	MaintenanceError string                     `json:"maintenance_error,omitempty"`
	Storage          runtime.Status             `json:"storage"`
	StorageError     string                     `json:"storage_error,omitempty"`
}

type Host struct {
	store           *runtime.Store
	consumer        *runtime.Consumer
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
}

var _ runner.Source = (*Host)(nil)

// New acquires the installation and its process protection atomically, once.
// Neither monitor changes nor subsequent installations change this selection.
func New(config Config) *Host {
	h := &Host{
		store:       config.Store,
		maintenance: make(map[string]error), log: logger.New(),
		state: StateMissing, done: make(chan struct{}), sweepInterval: time.Hour,
	}
	err := config.Unavailable
	if err == nil && h.store == nil {
		err = errors.New("browser runtime store is unavailable")
	}
	if err == nil {
		h.consumer, err = h.store.AcquireSelected()
	}
	if err != nil {
		h.state, h.setupErr = StateUnavailable, err
		if errors.Is(err, runtime.ErrUnavailable) {
			h.state = StateMissing
			h.setupErr = fmt.Errorf("install a browser runtime and restart dem.plugin: %w", err)
		}
		return h
	}
	manifest := h.consumer.Manifest()
	h.identity = &synthetic.RuntimeIdentity{Version: manifest.Version, SHA256: manifest.Archive.SHA256, Contract: manifest.Contract}
	h.state = StateReady
	return h
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
	result := Snapshot{State: h.state, SetupError: errorText(h.setupErr), MaintenanceError: errorText(h.maintenanceErrorLocked())}
	if h.identity != nil {
		identity := *h.identity
		result.Running = &identity
	}
	h.mu.Unlock()
	if h.store != nil {
		var err error
		result.Storage, err = h.store.Inspect()
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

type managedUse struct {
	use      *runtime.Use
	host     *Host
	identity synthetic.RuntimeIdentity
}

func (u managedUse) Paths() runner.Paths {
	return runner.Paths{Node: u.use.Paths.Node, EntryPoint: u.use.Paths.EntryPoint, Identity: u.identity}
}
func (u managedUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u managedUse) Finish(drained bool) error {
	err := u.use.Finish(drained)
	if err != nil {
		u.host.recordFinishError(err)
	}
	return err
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
	if h.store == nil {
		return
	}
	h.mu.Lock()
	version := h.finishVersion
	h.mu.Unlock()
	err := h.store.Sweep()
	h.setMaintenance("cleanup", err)
	h.mu.Lock()
	// A newer Finish failure must not be cleared by an older sweep.
	var change healthChange
	if err == nil && version == h.finishVersion {
		change = h.updateMaintenanceLocked("execution completion", nil)
	}
	h.mu.Unlock()
	h.logHealth(change)
}

// Close follows joined jobs, releases process protection, and leaves the
// caller-supplied Store open. Uncertain executions retain their own receipts.
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
