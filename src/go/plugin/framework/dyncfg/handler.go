// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// Callbacks defines component-specific operations for the handler.
type Callbacks[C Config] interface {
	// ExtractKey parses dyncfg function ID into cache key + config name.
	ExtractKey(fn Function) (key, name string, ok bool)

	// ParseAndValidate parses payload into a config with dyncfg metadata set.
	// Includes all validation (including heavy checks like module instantiation).
	ParseAndValidate(fn Function, name string) (C, error)

	// ValidateConfigName enforces the domain's config-name policy. Called before
	// ParseAndValidate so cheap name-format rejections happen without parsing payload.
	ValidateConfigName(name string) error

	// Start creates a work unit and starts it. Owns the full start lifecycle
	// including pre-start cleanup and post-fail retry scheduling.
	// Return CodedError to override the default 422 failure response.
	// Used by CmdEnable and CmdUpdate (conversion only).
	Start(cfg C) error

	// Update handles non-conversion config updates (dyncfg->dyncfg).
	// Called after caches are already updated; the callback owns the runtime
	// transition semantics.
	Update(oldCfg, newCfg C) error

	// Stop stops all work and cleans up all component state for a config.
	// Safe to call for non-running configs (all ops are no-ops).
	Stop(cfg C)

	// OnStatusChange is called after status transitions in enable/disable/update.
	// Not called in CmdAdd or CmdRemove.
	OnStatusChange(entry *Entry[C], oldStatus Status, fn Function)

	// ConfigID returns the dyncfg wire protocol ID for a config.
	ConfigID(cfg C) string

	// ConfigType returns the dyncfg type for a config.
	ConfigType(cfg C) ConfigType
}

// CodedError allows callbacks to override the default response code.
type CodedError interface {
	error
	DyncfgCode() int
}

// RetryableError marks errors that should keep job auto-detection retry enabled.
// The method name is intentionally Netdata-specific to avoid matching unrelated
// dependency errors that happen to expose Retryable() bool.
type RetryableError interface {
	error
	DyncfgRetryable() bool
}

func IsRetryableError(err error) bool {
	var re RetryableError
	return errors.As(err, &re) && re.DyncfgRetryable()
}

// HandlerOpts configures the handler with component-specific settings.
type HandlerOpts[C Config] struct {
	API       *Responder
	Seen      *SeenCache[C]
	Exposed   *ExposedCache[C]
	Callbacks Callbacks[C]
	WaitKey   func(cfg C) string // optional key used to gate config processing until enable/disable

	Path           string    // dyncfg path (e.g. "/collectors/go.d/Jobs")
	ConfigCommands []Command // base commands for non-template configs; CommandRemove is added implicitly only for dyncfg ConfigTypeJob configs
}

// Handler implements the shared dyncfg command state machine.
// It manages two caches (seen/exposed) borrowed from the component,
// and delegates domain-specific work to Callbacks.
type Handler[C Config] struct {
	api            *Responder
	seen           *SeenCache[C]
	exposed        *ExposedCache[C]
	cb             Callbacks[C]
	path           string
	configCommands []Command
	waitGate       *waitGate[C]
}

// waitGate is mutated by the serialized service-discovery event loop and may
// be observed by control-plane callers.
type waitGate[C Config] struct {
	keyFn func(cfg C) string
	mu    sync.RWMutex
	key   string
}

func newWaitGate[C Config](keyFn func(cfg C) string) *waitGate[C] {
	return &waitGate[C]{keyFn: keyFn}
}

func (wg *waitGate[C]) waitForDecision(cfg C) {
	if wg.keyFn == nil {
		return
	}
	key := wg.keyFn(cfg)
	if key == "" {
		return
	}

	wg.mu.Lock()
	wg.key = key
	wg.mu.Unlock()
}

func (wg *waitGate[C]) waitingForDecision() bool {
	wg.mu.RLock()
	waiting := wg.key != ""
	wg.mu.RUnlock()
	return waiting
}

func (wg *waitGate[C]) nextDecision(ctx context.Context, dyncfgCh <-chan Function) (Function, bool) {
	select {
	case <-ctx.Done():
		return Function{}, false
	case fn := <-dyncfgCh:
		return fn, true
	}
}

func (wg *waitGate[C]) currentKey() string {
	wg.mu.RLock()
	key := wg.key
	wg.mu.RUnlock()
	return key
}

func (wg *waitGate[C]) keyFor(cfg C) string {
	if wg.keyFn == nil {
		return ""
	}
	return wg.keyFn(cfg)
}

func (wg *waitGate[C]) clearIfMatch(key string) {
	wg.mu.Lock()
	defer wg.mu.Unlock()

	if wg.key == key {
		wg.key = ""
	}
}

func NewHandler[C Config](opts HandlerOpts[C]) *Handler[C] {
	return &Handler[C]{
		api:            opts.API,
		seen:           opts.Seen,
		exposed:        opts.Exposed,
		cb:             opts.Callbacks,
		path:           opts.Path,
		configCommands: opts.ConfigCommands,
		waitGate:       newWaitGate(opts.WaitKey),
	}
}

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
	h.waitGate.waitForDecision(cfg)
}

// WaitingForDecision reports whether config processing should currently wait
// for a matching enable/disable command.
func (h *Handler[C]) WaitingForDecision() bool {
	return h.waitGate.waitingForDecision()
}

// NextWaitDecision blocks until a dyncfg command arrives or ctx is canceled.
func (h *Handler[C]) NextWaitDecision(ctx context.Context, dyncfgCh <-chan Function) (Function, bool) {
	return h.waitGate.nextDecision(ctx, dyncfgCh)
}

// SyncDecision updates wait-state based on the incoming command.
// Only a matching enable/disable command clears the current wait key.
func (h *Handler[C]) SyncDecision(fn Function) {
	cmd := fn.Command()
	if cmd != CommandEnable && cmd != CommandDisable {
		return
	}

	waitKey := h.waitGate.currentKey()
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
	if h.waitGate.keyFor(entry.Cfg) != waitKey {
		return
	}

	h.waitGate.clearIfMatch(waitKey)
}

// NotifyConfigCreate registers/updates a config in the dyncfg API (upsert).
func (h *Handler[C]) NotifyConfigCreate(cfg C, status Status) {
	isDyncfg := cfg.SourceType() == "dyncfg"
	h.api.ConfigCreate(netdataapi.ConfigOpts{
		ID:                h.cb.ConfigID(cfg),
		Status:            status.String(),
		ConfigType:        h.cb.ConfigType(cfg).String(),
		Path:              h.path,
		SourceType:        cfg.SourceType(),
		Source:            cfg.Source(),
		SupportedCommands: h.configSupportedCommands(cfg, isDyncfg),
	})
}

// NotifyConfigStatus sends a status update for a config.
func (h *Handler[C]) NotifyConfigStatus(cfg C, status Status) {
	h.api.ConfigStatus(h.cb.ConfigID(cfg), status)
}

// NotifyConfigRemove removes a config from the dyncfg API.
func (h *Handler[C]) NotifyConfigRemove(cfg C) {
	h.api.ConfigDelete(h.cb.ConfigID(cfg))
}

func (h *Handler[C]) configSupportedCommands(cfg C, isDyncfg bool) string {
	cmds := make([]Command, 0, len(h.configCommands)+1)
	for _, cmd := range h.configCommands {
		if cmd == CommandAdd || cmd == CommandRemove {
			continue
		}
		cmds = append(cmds, cmd)
	}
	if isDyncfg && h.cb.ConfigType(cfg) == ConfigTypeJob {
		cmds = append(cmds, CommandRemove)
	}
	return JoinCommands(cmds...)
}

// CmdAdd handles the "add" command.
func (h *Handler[C]) CmdAdd(fn Function) {
	key, name, code, msg := h.addRejection(fn)
	if code != 0 {
		h.api.SendCodef(fn, code, "%s", msg)
		return
	}

	newCfg, err := h.cb.ParseAndValidate(fn, name)
	if err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}
	if h.cb.ConfigType(newCfg) != ConfigTypeJob {
		h.api.SendCodef(fn, 405, "adding configurations of type '%s' is not supported, only 'job' configurations can be added.", h.cb.ConfigType(newCfg))
		return
	}

	if existing, ok := h.exposed.LookupByKey(key); ok {
		if _, found := h.seen.Lookup(existing.Cfg); found && existing.Cfg.SourceType() == "dyncfg" {
			h.seen.Remove(existing.Cfg)
		}
		h.exposed.Remove(existing.Cfg)
		h.cb.Stop(existing.Cfg)
	}

	h.seen.Add(newCfg)
	h.exposed.Add(&Entry[C]{Cfg: newCfg, Status: StatusAccepted})
	h.api.SendCodef(fn, 202, "")
	h.NotifyConfigCreate(newCfg, StatusAccepted)
}

// CmdEnable handles the "enable" command.
func (h *Handler[C]) CmdEnable(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid config ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "config not found.")
		return
	}

	oldStatus := entry.Status

	switch entry.Status {
	case StatusRunning:
		h.api.SendCodef(fn, 200, "")
		h.NotifyConfigStatus(entry.Cfg, StatusRunning)
		return
	case StatusAccepted, StatusDisabled, StatusFailed:
		// proceed to start
	default:
		h.api.SendCodef(fn, 405, "enabling is not allowed in '%s' state.", entry.Status)
		h.NotifyConfigStatus(entry.Cfg, entry.Status)
		return
	}

	err := h.cb.Start(entry.Cfg)
	if err != nil {
		entry.Status = StatusFailed

		code := 422
		if ce, ok := errors.AsType[CodedError](err); ok {
			code = ce.DyncfgCode()
		}
		h.api.SendCodef(fn, code, "%v", err)
		h.NotifyConfigStatus(entry.Cfg, StatusFailed)

		h.cb.OnStatusChange(entry, oldStatus, fn)
		return
	}

	entry.Status = StatusRunning
	h.api.SendCodef(fn, 200, "")
	h.NotifyConfigStatus(entry.Cfg, StatusRunning)
	h.cb.OnStatusChange(entry, oldStatus, fn)
}

// CmdDisable handles the "disable" command.
func (h *Handler[C]) CmdDisable(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid config ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "config not found.")
		return
	}

	oldStatus := entry.Status

	if entry.Status == StatusDisabled {
		h.api.SendCodef(fn, 200, "")
		h.NotifyConfigStatus(entry.Cfg, StatusDisabled)
		return
	}

	// Unconditional for all non-Disabled statuses.
	h.cb.Stop(entry.Cfg)
	entry.Status = StatusDisabled
	h.api.SendCodef(fn, 200, "")
	h.NotifyConfigStatus(entry.Cfg, StatusDisabled)
	h.cb.OnStatusChange(entry, oldStatus, fn)
}

// CmdRemove handles the "remove" command.
func (h *Handler[C]) CmdRemove(fn Function) {
	key, _, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid config ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "config not found.")
		return
	}

	if entry.Cfg.SourceType() != "dyncfg" {
		h.api.SendCodef(fn, 405, "removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.", entry.Cfg.SourceType())
		return
	}
	if h.cb.ConfigType(entry.Cfg) != ConfigTypeJob {
		h.api.SendCodef(fn, 405, "removing configurations of type '%s' is not supported, only 'job' configurations can be removed.", h.cb.ConfigType(entry.Cfg))
		return
	}

	h.seen.Remove(entry.Cfg)
	h.exposed.Remove(entry.Cfg)
	h.cb.Stop(entry.Cfg)
	h.api.SendCodef(fn, 200, "")
	h.NotifyConfigRemove(entry.Cfg)
}

// CmdUpdate handles the "update" command.
func (h *Handler[C]) CmdUpdate(fn Function) {
	name, entry, code, msg := h.updateRejection(fn)
	if code != 0 {
		h.api.SendCodef(fn, code, "%s", msg)
		return
	}

	newCfg, err := h.cb.ParseAndValidate(fn, name)
	if err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		h.NotifyConfigStatus(entry.Cfg, entry.Status)
		return
	}

	isConversion := entry.Cfg.SourceType() != "dyncfg"
	if !isConversion && entry.Status == StatusRunning && entry.Cfg.Hash() == newCfg.Hash() {
		h.api.SendCodef(fn, 200, "")
		h.NotifyConfigStatus(entry.Cfg, StatusRunning)
		return
	}
	if entry.Status == StatusAccepted {
		h.api.SendCodef(fn, 403, "updating is not allowed in '%s' state.", entry.Status)
		h.NotifyConfigStatus(entry.Cfg, StatusAccepted)
		return
	}

	oldStatus := entry.Status
	oldCfg := entry.Cfg
	if isConversion {
		h.cb.Stop(oldCfg)
	} else {
		h.seen.Remove(oldCfg)
	}

	h.seen.Add(newCfg)
	newEntry := &Entry[C]{Cfg: newCfg, Status: StatusAccepted}
	h.exposed.Add(newEntry)

	if oldStatus == StatusDisabled {
		newEntry.Status = StatusDisabled
		if isConversion {
			h.NotifyConfigCreate(newCfg, StatusDisabled)
		}
		h.api.SendCodef(fn, 200, "")
		h.NotifyConfigStatus(newCfg, StatusDisabled)
		h.cb.OnStatusChange(newEntry, oldStatus, fn)
		return
	}

	if isConversion {
		err = h.cb.Start(newCfg)
	} else {
		err = h.cb.Update(oldCfg, newCfg)
	}
	if err != nil {
		if !isConversion && errors.Is(err, ErrNonDisruptiveUpdate) {
			h.seen.Remove(newCfg)
			h.seen.Add(oldCfg)
			h.exposed.Add(entry)
			h.api.SendCodef(fn, 200, "%v", err)
			h.NotifyConfigStatus(oldCfg, oldStatus)
			return
		}

		newEntry.Status = StatusFailed
		if isConversion {
			h.NotifyConfigCreate(newCfg, StatusFailed)
		}
		code := 200
		if ce, ok := errors.AsType[CodedError](err); ok {
			code = ce.DyncfgCode()
		}
		h.api.SendCodef(fn, code, "%v", err)
		h.NotifyConfigStatus(newCfg, StatusFailed)
		h.cb.OnStatusChange(newEntry, oldStatus, fn)
		return
	}

	newEntry.Status = StatusRunning
	if isConversion {
		h.NotifyConfigCreate(newCfg, StatusRunning)
	}
	h.api.SendCodef(fn, 200, "")
	h.NotifyConfigStatus(newCfg, StatusRunning)
	h.cb.OnStatusChange(newEntry, oldStatus, fn)
}

// addRejection runs CmdAdd's deterministic pre-validation gates.
func (h *Handler[C]) addRejection(fn Function) (key, name string, code int, msg string) {
	if err := fn.ValidateArgs(3); err != nil {
		return "", "", 400, fmt.Sprintf("%v", err)
	}
	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		return "", "", 400, "invalid config ID format."
	}
	if err := fn.ValidateHasPayload(); err != nil {
		return "", "", 400, fmt.Sprintf("%v", err)
	}
	if err := h.cb.ValidateConfigName(name); err != nil {
		return "", "", 400, fmt.Sprintf("invalid config name '%s': %v.", name, err)
	}
	return key, name, 0, ""
}

// updateRejection runs CmdUpdate's deterministic pre-validation gates.
func (h *Handler[C]) updateRejection(fn Function) (name string, entry *Entry[C], code int, msg string) {
	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		return "", nil, 400, "invalid config ID format."
	}
	entry, ok = h.exposed.LookupByKey(key)
	if !ok {
		return "", nil, 404, "config not found."
	}
	if err := fn.ValidateHasPayload(); err != nil {
		return "", nil, 400, fmt.Sprintf("%v", err)
	}
	return name, entry, 0, ""
}
