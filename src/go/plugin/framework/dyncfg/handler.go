// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
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
	// Validation is structural only; construction belongs in PrepareUpdate or activation.
	ParseAndValidate(fn Function, name string) (C, error)

	// ValidateConfigName enforces the domain's config-name policy. Called before
	// ParseAndValidate so cheap name-format rejections happen without parsing payload.
	ValidateConfigName(name string) error

	// Enable adopts enabled intent without performing activation work. The returned
	// continuation runs only after the acceptance reply and status are published.
	// An error must leave the incumbent and enabled intent unchanged.
	Enable(cfg C) (func(), error)

	// PrepareUpdate validates an enabled replacement while preserving the incumbent.
	// It honors fn.Context() and owns cleanup on error; success transfers the
	// returned activation to the handler for acceptance or disposal.
	PrepareUpdate(fn Function, oldCfg, newCfg C) (PreparedActivation, error)

	// Stop logically revokes work without waiting for physical cleanup.
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

// CodedError overrides the response code when a callback error becomes a rejection.
// Handler uses it for ParseAndValidate and PrepareUpdate. Apply errors propagate
// to the managed caller without a result, regardless of CodedError.
type CodedError interface {
	error
	DyncfgCode() int
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
	return &waitGate[C]{
		keyFn: keyFn,
	}
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

func (wg *waitGate[C]) replace(oldCfg, newCfg C) {
	oldKey, newKey := wg.keyFor(oldCfg), wg.keyFor(newCfg)
	wg.mu.Lock()
	defer wg.mu.Unlock()
	if wg.key != "" && wg.key == oldKey {
		wg.key = newKey
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
	entry := &Entry[C]{
		Cfg:     cfg,
		Status:  status,
		Enabled: status == StatusRunning || status == StatusFailed,
	}
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
	h.api.ConfigCreate(h.configCreateOpts(cfg, status))
}

// ValidateConfigCreate checks the exact CONFIG create frame generated for cfg.
func (h *Handler[C]) ValidateConfigCreate(cfg C, status Status) error {
	return h.configCreateOpts(cfg, status).Validate()
}

func (h *Handler[C]) configCreateOpts(cfg C, status Status) netdataapi.ConfigOpts {
	isDyncfg := cfg.SourceType() == "dyncfg"
	return netdataapi.ConfigOpts{
		ID:                h.cb.ConfigID(cfg),
		Status:            status.String(),
		ConfigType:        h.cb.ConfigType(cfg).String(),
		Path:              h.path,
		SourceType:        cfg.SourceType(),
		Source:            cfg.Source(),
		SupportedCommands: h.configSupportedCommands(cfg, isDyncfg),
	}
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

// Direct command wrappers serve standalone users; managed callers use Prepare.
func (h *Handler[C]) CmdAdd(fn Function)     { h.execute(fn) }
func (h *Handler[C]) CmdEnable(fn Function)  { h.execute(fn) }
func (h *Handler[C]) CmdDisable(fn Function) { h.execute(fn) }
func (h *Handler[C]) CmdRemove(fn Function)  { h.execute(fn) }
func (h *Handler[C]) CmdUpdate(fn Function)  { h.execute(fn) }

func (h *Handler[C]) execute(fn Function) {
	prepared, err := h.Prepare(fn)
	if err != nil {
		h.api.SendCodef(fn, 500, "%v", err)
		return
	}
	applied, err := prepared.Apply(fn.Context())
	if err != nil {
		h.api.SendCodef(fn, 500, "%v", err)
		return
	}
	if fn.UID() != "" {
		h.api.output.FunctionResult(applied.Result)
	}
	for _, notification := range applied.Notifications {
		notification.Emit(h.api.output)
	}
	if applied.Published != nil {
		applied.Published()
	}
}

// SetStatus replaces the current immutable snapshot after the caller has checked
// its runtime generation. Config identity prevents publication for replaced input.
// It returns true only when the status changed; duplicate events retain the snapshot.
func (h *Handler[C]) SetStatus(cfg C, status Status) bool {
	entry, ok := h.exposed.LookupByKey(cfg.ExposedKey())
	if !ok || entry.Cfg.UID() != cfg.UID() || entry.Cfg.Hash() != cfg.Hash() || entry.Status == status {
		return false
	}
	h.exposed.Add(&Entry[C]{
		Cfg:     entry.Cfg,
		Status:  status,
		Enabled: entry.Enabled,
	})
	return true
}

func callbackErrorCode(err error, fallback int) int {
	if ce, ok := errors.AsType[CodedError](err); ok && ce.DyncfgCode() >= 400 && ce.DyncfgCode() < 600 {
		return ce.DyncfgCode()
	}
	return fallback
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
