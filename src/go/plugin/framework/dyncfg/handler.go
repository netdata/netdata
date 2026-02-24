// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// Callbacks defines component-specific operations for the handler.
type Callbacks[C Config] interface {
	// ExtractKey parses dyncfg function ID into cache key + job name.
	ExtractKey(fn Function) (key, name string, ok bool)

	// ParseAndValidate parses payload into a config with dyncfg metadata set.
	// Includes all validation (including heavy checks like module instantiation).
	ParseAndValidate(fn Function, name string) (C, error)

	// Start creates a work unit and starts it. Owns the full start lifecycle
	// including pre-start cleanup and post-fail retry scheduling.
	// Return CodedError to override EnableFailCode.
	// Used by CmdEnable and CmdUpdate (conversion only).
	Start(cfg C) error

	// Update handles non-conversion config updates (dyncfg->dyncfg).
	// Called after caches are already updated. SD uses mgr.Restart
	// for graceful transition; jobmgr uses Stop+Start.
	Update(oldCfg, newCfg C) error

	// Stop stops all work and cleans up all component state for a config.
	// Safe to call for non-running configs (all ops are no-ops).
	Stop(cfg C)

	// OnStatusChange is called after status transitions in enable/disable/update.
	// Not called in CmdAdd or CmdRemove.
	OnStatusChange(entry *Entry[C], oldStatus Status, fn Function)

	// ConfigID returns the dyncfg wire protocol ID for a config.
	ConfigID(cfg C) string
}

// CodedError allows callbacks to override the default response code.
type CodedError interface {
	error
	Code() int
}

// HandlerOpts configures the handler with component-specific settings.
type HandlerOpts[C Config] struct {
	Logger    *logger.Logger
	API       *Responder
	Seen      *SeenCache[C]
	Exposed   *ExposedCache[C]
	Callbacks Callbacks[C]
	WaitKey   func(cfg C) string // optional key used to gate config processing until enable/disable

	Path                    string    // dyncfg path (e.g. "/collectors/go.d/Jobs")
	EnableFailCode          int       // response code for enable failure (jobmgr: 200, SD: 422)
	RemoveStockOnEnableFail bool      // remove stock config from exposed on enable failure
	JobCommands             []Command // base commands for jobs; CommandRemove is added implicitly for dyncfg configs
}

// Handler implements the shared dyncfg command state machine.
// It manages two caches (seen/exposed) borrowed from the component,
// and delegates domain-specific work to Callbacks.
type Handler[C Config] struct {
	*logger.Logger
	api                     *Responder
	seen                    *SeenCache[C]
	exposed                 *ExposedCache[C]
	cb                      Callbacks[C]
	path                    string
	enableFailCode          int
	removeStockOnEnableFail bool
	jobCommands             []Command
	waitKeyFn               func(cfg C) string
	waitKey                 string
	waitMu                  sync.RWMutex
}

func NewHandler[C Config](opts HandlerOpts[C]) *Handler[C] {
	return &Handler[C]{
		Logger:                  opts.Logger,
		api:                     opts.API,
		seen:                    opts.Seen,
		exposed:                 opts.Exposed,
		cb:                      opts.Callbacks,
		path:                    opts.Path,
		enableFailCode:          opts.EnableFailCode,
		removeStockOnEnableFail: opts.RemoveStockOnEnableFail,
		jobCommands:             opts.JobCommands,
		waitKeyFn:               opts.WaitKey,
	}
}

func (h *Handler[C]) Seen() *SeenCache[C]       { return h.seen }
func (h *Handler[C]) Exposed() *ExposedCache[C] { return h.exposed }

// SetAPI replaces the responder (e.g. to silence output in CLI mode).
func (h *Handler[C]) SetAPI(api *Responder) { h.api = api }

// RememberDiscoveredConfig ensures a discovered config is present in Seen cache.
func (h *Handler[C]) RememberDiscoveredConfig(cfg C) {
	if _, ok := h.seen.Lookup(cfg); ok {
		return
	}
	h.seen.Add(cfg)
}

// AddDiscoveredConfig upserts a discovered config into Seen and Exposed caches.
func (h *Handler[C]) AddDiscoveredConfig(cfg C, status Status) *Entry[C] {
	h.RememberDiscoveredConfig(cfg)
	entry := &Entry[C]{Cfg: cfg, Status: status}
	h.exposed.Add(entry)
	return entry
}

// RemoveDiscoveredConfig removes a discovered config from Seen and Exposed caches.
// Returns the removed Exposed entry when the removed seen config was also exposed.
func (h *Handler[C]) RemoveDiscoveredConfig(cfg C) (*Entry[C], bool) {
	if _, ok := h.seen.Lookup(cfg); !ok {
		return nil, false
	}
	h.seen.Remove(cfg)

	entry, ok := h.exposed.LookupByKey(cfg.ExposedKey())
	if !ok || entry.Cfg.UID() != cfg.UID() {
		return nil, false
	}

	h.exposed.Remove(cfg)
	return entry, true
}

// WaitForDecision blocks non-dyncfg config processing until a matching
// enable/disable command is observed for the provided config.
func (h *Handler[C]) WaitForDecision(cfg C) {
	if h.waitKeyFn == nil {
		return
	}
	key := h.waitKeyFn(cfg)
	if key == "" {
		return
	}
	h.waitMu.Lock()
	h.waitKey = key
	h.waitMu.Unlock()
}

// WaitingForDecision reports whether config processing should currently wait
// for a matching enable/disable command.
func (h *Handler[C]) WaitingForDecision() bool {
	h.waitMu.RLock()
	defer h.waitMu.RUnlock()
	return h.waitKey != ""
}

// SyncDecision updates wait-state based on the incoming command.
// Only a matching enable/disable command clears the current wait key.
func (h *Handler[C]) SyncDecision(fn Function) {
	if h.waitKeyFn == nil {
		return
	}
	cmd := fn.Command()
	if cmd != CommandEnable && cmd != CommandDisable {
		return
	}

	h.waitMu.RLock()
	waitKey := h.waitKey
	h.waitMu.RUnlock()
	if waitKey == "" {
		return
	}

	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		return
	}
	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		return
	}
	if h.waitKeyFn(entry.Cfg) != waitKey {
		return
	}

	h.waitMu.Lock()
	if h.waitKey == waitKey {
		h.waitKey = ""
	}
	h.waitMu.Unlock()
}

// NotifyJobCreate registers/updates a config in the dyncfg API (upsert).
func (h *Handler[C]) NotifyJobCreate(cfg C, status Status) {
	isDyncfg := cfg.SourceType() == "dyncfg"
	h.api.ConfigCreate(netdataapi.ConfigOpts{
		ID:                h.cb.ConfigID(cfg),
		Status:            status.String(),
		ConfigType:        ConfigTypeJob.String(),
		Path:              h.path,
		SourceType:        cfg.SourceType(),
		Source:            cfg.Source(),
		SupportedCommands: h.jobSupportedCommands(isDyncfg),
	})
}

// NotifyJobStatus sends a status update for a config.
func (h *Handler[C]) NotifyJobStatus(cfg C, status Status) {
	h.api.ConfigStatus(h.cb.ConfigID(cfg), status)
}

// NotifyJobRemove removes a config from the dyncfg API.
func (h *Handler[C]) NotifyJobRemove(cfg C) {
	h.api.ConfigDelete(h.cb.ConfigID(cfg))
}

func (h *Handler[C]) jobSupportedCommands(isDyncfg bool) string {
	cmds := make([]Command, len(h.jobCommands))
	copy(cmds, h.jobCommands)
	if isDyncfg {
		cmds = append(cmds, CommandRemove)
	}
	return JoinCommands(cmds...)
}

// CmdAdd handles the "add" command.
func (h *Handler[C]) CmdAdd(fn Function) {
	if err := fn.ValidateArgs(3); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	if err := ValidateJobName(name); err != nil {
		h.api.SendCodef(fn, 400, "invalid job name '%s': %v.", name, err)
		return
	}

	newCfg, err := h.cb.ParseAndValidate(fn, name)
	if err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	// Replace existing config at the same key, if any.
	if existing, ok := h.exposed.LookupByKey(key); ok {
		if _, found := h.seen.Lookup(existing.Cfg); found && existing.Cfg.SourceType() == "dyncfg" {
			h.seen.Remove(existing.Cfg)
		}
		h.exposed.Remove(existing.Cfg)
		h.cb.Stop(existing.Cfg)
	}

	h.seen.Add(newCfg)
	newEntry := &Entry[C]{Cfg: newCfg, Status: StatusAccepted}
	h.exposed.Add(newEntry)

	h.api.SendCodef(fn, 202, "")
	h.NotifyJobCreate(newCfg, StatusAccepted)
}

// CmdEnable handles the "enable" command.
func (h *Handler[C]) CmdEnable(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "job not found.")
		return
	}

	oldStatus := entry.Status

	switch entry.Status {
	case StatusRunning:
		h.api.SendCodef(fn, 200, "")
		h.NotifyJobStatus(entry.Cfg, StatusRunning)
		return
	case StatusAccepted, StatusDisabled, StatusFailed:
		// proceed to start
	default:
		h.api.SendCodef(fn, 405, "enabling is not allowed in '%s' state.", entry.Status)
		h.NotifyJobStatus(entry.Cfg, entry.Status)
		return
	}

	err := h.cb.Start(entry.Cfg)

	if err != nil {
		entry.Status = StatusFailed

		code := h.enableFailCode
		var ce CodedError
		if errors.As(err, &ce) {
			code = ce.Code()
		}
		h.api.SendCodef(fn, code, "%v", err)

		// Stock removal only for non-CodedError failures (runtime detection failures).
		// CodedError = validation error (e.g. createCollectorJob → 400, no stock removal).
		if h.removeStockOnEnableFail && !isCodedError(err) && entry.Cfg.SourceType() == "stock" {
			h.exposed.Remove(entry.Cfg)
			h.NotifyJobRemove(entry.Cfg)
		} else {
			h.NotifyJobStatus(entry.Cfg, StatusFailed)
		}

		h.cb.OnStatusChange(entry, oldStatus, fn)
		return
	}

	entry.Status = StatusRunning
	h.api.SendCodef(fn, 200, "")
	h.NotifyJobStatus(entry.Cfg, StatusRunning)
	h.cb.OnStatusChange(entry, oldStatus, fn)
}

// CmdDisable handles the "disable" command.
func (h *Handler[C]) CmdDisable(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "job not found.")
		return
	}

	oldStatus := entry.Status

	if entry.Status == StatusDisabled {
		h.api.SendCodef(fn, 200, "")
		h.NotifyJobStatus(entry.Cfg, StatusDisabled)
		return
	}

	// Unconditional for all non-Disabled statuses.
	h.cb.Stop(entry.Cfg)

	entry.Status = StatusDisabled
	h.api.SendCodef(fn, 200, "")
	h.NotifyJobStatus(entry.Cfg, StatusDisabled)
	h.cb.OnStatusChange(entry, oldStatus, fn)
}

// CmdRemove handles the "remove" command.
func (h *Handler[C]) CmdRemove(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "job not found.")
		return
	}

	if entry.Cfg.SourceType() != "dyncfg" {
		h.api.SendCodef(fn, 405, "removing jobs of type '%s' is not supported, only 'dyncfg' jobs can be removed.", entry.Cfg.SourceType())
		return
	}

	h.seen.Remove(entry.Cfg)
	h.exposed.Remove(entry.Cfg)
	h.cb.Stop(entry.Cfg)

	h.api.SendCodef(fn, 200, "")
	h.NotifyJobRemove(entry.Cfg)
}

// CmdUpdate handles the "update" command.
func (h *Handler[C]) CmdUpdate(fn Function) {
	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "job not found.")
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	newCfg, err := h.cb.ParseAndValidate(fn, name)
	if err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		h.NotifyJobStatus(entry.Cfg, entry.Status)
		return
	}

	isConversion := entry.Cfg.SourceType() != "dyncfg"

	// No-op: running dyncfg config with same hash.
	if !isConversion && entry.Status == StatusRunning && entry.Cfg.Hash() == newCfg.Hash() {
		h.api.SendCodef(fn, 200, "")
		h.NotifyJobStatus(entry.Cfg, StatusRunning)
		return
	}

	if entry.Status == StatusAccepted {
		h.api.SendCodef(fn, 403, "updating is not allowed in '%s' state.", entry.Status)
		h.NotifyJobStatus(entry.Cfg, StatusAccepted)
		return
	}

	oldStatus := entry.Status
	oldCfg := entry.Cfg

	// For conversion: stop old before cache update (matching jobmgr line 681).
	if isConversion {
		h.cb.Stop(oldCfg)
	}

	// Update caches.
	if !isConversion {
		h.seen.Remove(oldCfg)
	}
	h.seen.Add(newCfg)
	newEntry := &Entry[C]{Cfg: newCfg, Status: StatusAccepted}
	h.exposed.Add(newEntry)

	// Preserve Disabled status.
	if oldStatus == StatusDisabled {
		newEntry.Status = StatusDisabled
		if isConversion {
			h.NotifyJobCreate(newCfg, StatusDisabled)
		}
		h.api.SendCodef(fn, 200, "")
		h.NotifyJobStatus(newCfg, StatusDisabled)
		h.cb.OnStatusChange(newEntry, oldStatus, fn)
		return
	}

	// Start or update.
	if isConversion {
		err = h.cb.Start(newCfg)
	} else {
		err = h.cb.Update(oldCfg, newCfg)
	}

	if err != nil {
		newEntry.Status = StatusFailed
		if isConversion {
			h.NotifyJobCreate(newCfg, StatusFailed)
		}
		h.api.SendCodef(fn, 200, "%v", err)
		h.NotifyJobStatus(newCfg, StatusFailed)
		h.cb.OnStatusChange(newEntry, oldStatus, fn)
		return
	}

	newEntry.Status = StatusRunning
	if isConversion {
		h.NotifyJobCreate(newCfg, StatusRunning)
	}
	h.api.SendCodef(fn, 200, "")
	h.NotifyJobStatus(newCfg, StatusRunning)
	h.cb.OnStatusChange(newEntry, oldStatus, fn)
}

// CmdRestart handles the "restart" command.
// Stops the existing work and starts the same config again.
// Only allowed for Running/Failed configs (rejects Accepted/Disabled).
func (h *Handler[C]) CmdRestart(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid job ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "job not found.")
		return
	}

	switch entry.Status {
	case StatusAccepted, StatusDisabled:
		h.api.SendCodef(fn, 405, "restarting is not allowed in '%s' state.", entry.Status)
		h.NotifyJobStatus(entry.Cfg, entry.Status)
		return
	case StatusRunning, StatusFailed:
		// proceed
	default:
		h.api.SendCodef(fn, 405, "restarting is not allowed in '%s' state.", entry.Status)
		h.NotifyJobStatus(entry.Cfg, entry.Status)
		return
	}

	oldStatus := entry.Status

	h.cb.Stop(entry.Cfg)

	err := h.cb.Start(entry.Cfg)

	if err != nil {
		entry.Status = StatusFailed
		code := 422
		var ce CodedError
		if errors.As(err, &ce) {
			code = ce.Code()
		}
		h.api.SendCodef(fn, code, "job restart failed: %v", err)
		h.NotifyJobStatus(entry.Cfg, StatusFailed)
		h.cb.OnStatusChange(entry, oldStatus, fn)
		return
	}

	entry.Status = StatusRunning
	h.api.SendCodef(fn, 200, "")
	h.NotifyJobStatus(entry.Cfg, StatusRunning)
	h.cb.OnStatusChange(entry, oldStatus, fn)
}

func isCodedError(err error) bool {
	var ce CodedError
	return errors.As(err, &ce)
}
