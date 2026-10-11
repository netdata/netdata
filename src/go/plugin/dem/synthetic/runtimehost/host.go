// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtimehost owns process-wide DEM runtime demand and preparation.
package runtimehost

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

const (
	DependencyKind   = "dem-runtime"
	DependencyName   = "browser"
	StateMissing     = "missing"
	StatePreparing   = "preparing"
	StateReady       = "ready"
	StateUnavailable = "unavailable"
)

// Config supplies installed trust and a caller-owned store. A missing source or
// store is reported as setup unavailability; it does not prevent RUM operation.
type Config struct {
	Store         *runtime.Store
	Manifest      *runtime.Manifest
	Unavailable   error
	AllowDownload bool
}

type Snapshot struct {
	Identity         string         `json:"identity,omitempty"`
	State            string         `json:"state"`
	Enabled          int            `json:"enabled"`
	DownloadAllowed  bool           `json:"download_allowed"`
	SetupError       string         `json:"setup_error,omitempty"`
	MaintenanceError string         `json:"maintenance_error,omitempty"`
	Storage          runtime.Status `json:"storage"`
	StorageError     string         `json:"storage_error,omitempty"`
}

type demand struct {
	claim     *runtime.Claim
	active    bool
	committed bool
	err       error
}

type Host struct {
	store                                      *runtime.Store
	manifest                                   *runtime.Manifest
	unavailable                                error
	allowDownload                              bool
	mu                                         sync.Mutex
	accepted                                   map[collectorapi.JobConfigIdentity][]*demand
	reservations                               map[*transition]*runtime.Claim
	retired                                    map[*runtime.Claim]struct{}
	subscribers                                map[uint64]func(string, string)
	nextSubscriber                             uint64
	state                                      string
	setupErr                                   error
	maintenance                                map[string]error
	finishVersion                              uint64
	reportedSetup                              string
	stopping                                   bool
	log                                        *logger.Logger
	started, closed                            bool
	cancel                                     context.CancelFunc
	done                                       chan struct{}
	wake                                       chan struct{}
	pollInterval, retryInterval, sweepInterval time.Duration
}

var _ collectorapi.JobConfigIntentOwner = (*Host)(nil)
var _ runner.Source = (*Host)(nil)

func New(config Config) *Host {
	h := &Host{
		store: config.Store, unavailable: config.Unavailable, allowDownload: config.AllowDownload,
		accepted: make(map[collectorapi.JobConfigIdentity][]*demand), reservations: make(map[*transition]*runtime.Claim),
		retired: make(map[*runtime.Claim]struct{}), subscribers: make(map[uint64]func(string, string)),
		maintenance: make(map[string]error), log: logger.New(),
		state: StateMissing, done: make(chan struct{}), wake: make(chan struct{}, 1),
		pollInterval: 5 * time.Second, retryInterval: time.Minute, sweepInterval: time.Hour,
	}
	if config.Manifest != nil {
		m := *config.Manifest
		m.Bindings = append([]runtime.Binding(nil), m.Bindings...)
		for i := range m.Bindings {
			m.Bindings[i].Expected = append([]byte(nil), m.Bindings[i].Expected...)
		}
		h.manifest = &m
		h.unavailable = errors.Join(h.unavailable, m.Validate())
	} else if h.unavailable == nil {
		h.unavailable = errors.New("no installed browser runtime manifest")
	}
	if h.store == nil && h.unavailable == nil {
		h.unavailable = errors.New("browser runtime store is unavailable")
	}
	if h.unavailable != nil {
		h.state, h.setupErr = StateUnavailable, h.unavailable
	}
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
	if h.state == StatePreparing {
		return runtime.ErrPreparing
	}
	return runtime.ErrUnavailable
}

func (h *Host) Dependencies(map[string]any) ([]collectorapi.ActivationDependency, error) {
	return []collectorapi.ActivationDependency{{Kind: DependencyKind, Name: DependencyName, Ready: h.Ready}}, nil
}

// SubscribeDependencies permits overlapping current and successor run owners.
// Callbacks run outside the host lock and must not block the process worker.
func (h *Host) SubscribeDependencies(notify func(kind, name string)) func() {
	h.mu.Lock()
	h.nextSubscriber++
	id := h.nextSubscriber
	if notify != nil {
		h.subscribers[id] = notify
	}
	h.mu.Unlock()
	return func() { h.mu.Lock(); delete(h.subscribers, id); h.mu.Unlock() }
}

func (h *Host) setState(state string, err error) {
	h.mu.Lock()
	changed := h.state != state || errorText(h.setupErr) != errorText(err)
	h.state, h.setupErr = state, err
	var callbacks []func(string, string)
	if changed {
		for _, callback := range h.subscribers {
			callbacks = append(callbacks, callback)
		}
	}
	setupMessage := ""
	if len(h.accepted) > 0 && !h.closed && !h.stopping {
		setupMessage = errorText(err)
	}
	logSetup := setupMessage != "" && setupMessage != h.reportedSetup
	h.reportedSetup = setupMessage
	h.mu.Unlock()
	if logSetup {
		h.log.Warningf("DEM runtime setup unavailable: %s", setupMessage)
	}
	for _, callback := range callbacks {
		callback(DependencyKind, DependencyName)
	}
}

func errorText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
func (h *Host) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *Host) Status() Snapshot {
	h.mu.Lock()
	result := Snapshot{State: h.state, DownloadAllowed: h.allowDownload, SetupError: errorText(h.setupErr), MaintenanceError: errorText(h.maintenanceErrorLocked())}
	if h.manifest != nil {
		result.Identity = h.manifest.Archive.SHA256
	}
	for _, entries := range h.accepted {
		result.Enabled += len(entries)
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
	use, err := h.store.BeginUse(*h.manifest)
	if err != nil {
		h.setState(StateUnavailable, err)
		h.signal()
		return nil, err
	}
	return managedUse{use: use, host: h}, nil
}

type managedUse struct {
	use  *runtime.Use
	host *Host
}

func (u managedUse) Paths() runner.Paths {
	return runner.Paths{Node: u.use.Paths.Node, Dependencies: u.use.Paths.Dependencies, Browser: u.use.Paths.Browser}
}
func (u managedUse) Completion() *ndexec.TreeCompletion { return u.use.Completion }
func (u managedUse) Finish(drained bool) error {
	err := u.use.Finish(drained)
	if err != nil {
		u.host.recordFinishError(err)
	}
	return err
}

// Run owns maintenance and one cancellable preparation. It handles operational
// errors locally and joins the downloader before returning on cancellation.
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
	poll := time.NewTicker(h.pollInterval)
	sweep := time.NewTicker(h.sweepInterval)
	defer poll.Stop()
	defer sweep.Stop()
	var download <-chan error
	var cancelDownload context.CancelFunc
	var retryAt time.Time
	var preparationErr error
	defer func() {
		if cancelDownload != nil {
			cancelDownload()
			<-download
		}
		h.mu.Lock()
		h.stopping = true
		h.mu.Unlock()
		h.setState(StateUnavailable, errors.New("browser runtime owner stopped"))
	}()
	h.sweep()
	h.signal()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			h.sweep()
		case <-poll.C:
		case <-h.wake:
		case err := <-download:
			cancelDownload()
			cancelDownload, download = nil, nil
			preparationErr = err
			retryAt = time.Now().Add(h.retryInterval)
		}
		if ctx.Err() != nil {
			return
		}
		enabled := h.reconcileClaims()
		if enabled == 0 && cancelDownload != nil {
			cancelDownload()
		}
		if h.unavailable != nil {
			h.setState(StateUnavailable, h.unavailable)
			continue
		}
		err := h.store.Available(*h.manifest)
		if err == nil {
			preparationErr = nil
			h.setState(StateReady, nil)
			continue
		}
		if !errors.Is(err, runtime.ErrUnavailable) {
			h.setState(StateUnavailable, err)
			continue
		}
		if download != nil {
			h.setState(StatePreparing, nil)
			continue
		}
		if enabled == 0 {
			h.setState(StateMissing, nil)
			continue
		}
		if !h.allowDownload {
			h.setState(StateUnavailable, errors.New("browser runtime is missing and automatic downloads are disabled"))
			continue
		}
		if h.manifest.Archive.URL == "" {
			h.setState(StateUnavailable, errors.New("browser runtime is missing and its installed manifest has no download URL"))
			continue
		}
		if time.Now().Before(retryAt) {
			if errors.Is(preparationErr, runtime.ErrPreparing) {
				h.setState(StatePreparing, nil)
			} else {
				h.setState(StateUnavailable, preparationErr)
			}
			continue
		}
		download, cancelDownload = h.startDownload(ctx)
		h.setState(StatePreparing, nil)
	}
}

func (h *Host) startDownload(ctx context.Context) (<-chan error, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		defer cancel()
		result <- h.store.Download(ctx, *h.manifest)
	}()
	return result, cancel
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
	// Sweep retries completed-use retirement. A newer Finish failure must not be
	// cleared by a sweep that began before that failure was recorded.
	recovered := err == nil && version == h.finishVersion
	var change healthChange
	if recovered {
		change = h.updateMaintenanceLocked("execution completion", nil)
	}
	h.mu.Unlock()
	h.logHealth(change)
}

func (h *Host) reconcileClaims() int {
	h.mu.Lock()
	var entries []*demand
	for _, active := range h.accepted {
		entries = append(entries, active...)
	}
	retired := make([]*runtime.Claim, 0, len(h.retired))
	for claim := range h.retired {
		retired = append(retired, claim)
	}
	h.mu.Unlock()
	for _, entry := range entries {
		h.mu.Lock()
		commit := entry.active && !entry.committed && entry.claim != nil
		h.mu.Unlock()
		if !commit {
			continue
		}
		err := entry.claim.Commit()
		h.mu.Lock()
		if entry.active {
			entry.err, entry.committed = err, err == nil
		}
		h.mu.Unlock()
	}
	var releaseErrors []error
	for _, claim := range retired {
		if err := claim.Close(); err == nil {
			h.mu.Lock()
			delete(h.retired, claim)
			h.mu.Unlock()
		} else {
			releaseErrors = append(releaseErrors, err)
		}
	}
	h.setMaintenance("demand release", stableErrors(releaseErrors))
	h.mu.Lock()
	var count int
	var commitErrors []error
	for _, active := range h.accepted {
		for _, entry := range active {
			count++
			if !entry.committed {
				if entry.err != nil {
					commitErrors = append(commitErrors, entry.err)
				}
			}
		}
	}
	result := stableErrors(commitErrors)
	change := h.updateMaintenanceLocked("demand commit", result)
	h.mu.Unlock()
	h.logHealth(change)
	return count
}

// Close is called after jobs and configuration transitions have joined. It joins
// Run, releases all owned claims, and leaves the caller-supplied Store open.
func (h *Host) Close() error {
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
	h.mu.Lock()
	claims := make(map[*runtime.Claim]struct{})
	for _, entries := range h.accepted {
		for _, entry := range entries {
			if entry.claim != nil {
				claims[entry.claim] = struct{}{}
			}
		}
	}
	for _, claim := range h.reservations {
		if claim != nil {
			claims[claim] = struct{}{}
		}
	}
	for claim := range h.retired {
		claims[claim] = struct{}{}
	}
	h.mu.Unlock()
	var result error
	for claim := range claims {
		result = errors.Join(result, claim.Close())
	}
	return result
}
