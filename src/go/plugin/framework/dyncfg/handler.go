// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

// Callbacks defines component-specific operations for the handler.
type Callbacks[C Config] interface {
	// ExtractKey parses dyncfg function ID into cache key + config name.
	ExtractKey(fn Function) (key, name string, ok bool)

	// ParseAndValidate parses payload into a config with dyncfg metadata set.
	// Includes all validation (including heavy checks like module instantiation).
	ParseAndValidate(ctx context.Context, fn Function, name string) (C, error)

	// ValidateConfigName enforces the domain's config-name policy. Called before
	// ParseAndValidate so cheap name-format rejections happen without parsing payload.
	ValidateConfigName(name string) error

	// Start creates a work unit and starts it. Owns the full start lifecycle
	// including pre-start cleanup and post-fail retry scheduling.
	// Return CodedError to override EnableFailCode.
	// Used by CmdEnable and CmdUpdate (conversion only).
	Start(ctx context.Context, cfg C) error

	// Update handles non-conversion config updates (dyncfg->dyncfg).
	// Called after caches are already updated. SD uses mgr.Restart
	// for graceful transition; jobmgr uses Stop+Start.
	Update(ctx context.Context, oldCfg, newCfg C) error

	// Stop stops all work and cleans up all component state for a config.
	// Safe to call for non-running configs (all ops are no-ops).
	Stop(ctx context.Context, cfg C)

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

// CommandMessageSource optionally provides a success/warning message
// for the command that just completed.
type CommandMessageSource interface {
	TakeCommandMessage() string
}

// HandlerOpts configures the handler with component-specific settings.
type HandlerOpts[C Config] struct {
	Logger      *logger.Logger
	API         *Responder
	Seen        *SeenCache[C]
	Exposed     *ExposedCache[C]
	Callbacks   Callbacks[C]
	WaitKey     func(cfg C) string // optional key used to gate config processing until enable/disable
	WaitTimeout time.Duration      // optional timeout for decision wait; zero keeps wait open until matching command

	Path                    string    // dyncfg path (e.g. "/collectors/go.d/Jobs")
	EnableFailCode          int       // response code for enable failure (jobmgr: 200, SD: 422)
	RemoveStockOnEnableFail bool      // remove stock config from exposed on enable failure
	ConfigCommands          []Command // base commands for non-template configs; CommandRemove is added implicitly only for dyncfg ConfigTypeJob configs
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
	configCommands          []Command
	waitGate                *waitGate[C]
}

func takeCommandMessage[C Config](cb Callbacks[C]) string {
	msgSrc, ok := any(cb).(CommandMessageSource)
	if !ok {
		return ""
	}
	return msgSrc.TakeCommandMessage()
}

// WaitTimeoutEvent describes a wait gate timeout transition.
type WaitTimeoutEvent struct {
	Key       string
	Elapsed   time.Duration
	Threshold time.Duration
}

// WaitDecisionStep is one serialized wait-loop transition.
type WaitDecisionStep struct {
	Command    Function
	HasCommand bool
	Timeout    WaitTimeoutEvent
	TimedOut   bool
}

// waitGate encapsulates wait-for-decision state and timing orchestration.
type waitGate[C Config] struct {
	keyFn    func(cfg C) string
	timeout  time.Duration
	key      string
	since    time.Time
	deadline time.Time
	mu       sync.RWMutex
	now      func() time.Time
}

func newWaitGate[C Config](keyFn func(cfg C) string, timeout time.Duration) *waitGate[C] {
	return &waitGate[C]{
		keyFn:   keyFn,
		timeout: timeout,
		now:     time.Now,
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
	wg.since = time.Time{}
	wg.deadline = time.Time{}
	if wg.timeout > 0 {
		now := wg.nowTime()
		wg.since = now
		wg.deadline = now.Add(wg.timeout)
	}
	wg.mu.Unlock()
}

func (wg *waitGate[C]) waitingForDecision() bool {
	wg.mu.RLock()
	defer wg.mu.RUnlock()
	return wg.key != ""
}

func (wg *waitGate[C]) decisionRemaining() (time.Duration, bool) {
	wg.mu.RLock()
	defer wg.mu.RUnlock()

	if wg.timeout <= 0 || wg.key == "" || wg.deadline.IsZero() {
		return 0, false
	}
	now := wg.nowTime()
	if now.After(wg.deadline) || now.Equal(wg.deadline) {
		return 0, true
	}
	return wg.deadline.Sub(now), true
}

func (wg *waitGate[C]) nextStep(ctx context.Context, dyncfgCh <-chan Function) (WaitDecisionStep, bool) {
	var step WaitDecisionStep

	waitFor, hasTimeout := wg.decisionRemaining()
	if !hasTimeout {
		select {
		case <-ctx.Done():
			return step, false
		case fn := <-dyncfgCh:
			step.Command = fn
			step.HasCommand = true
			return step, true
		}
	}

	timer := time.NewTimer(waitFor)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return step, false
	case fn := <-dyncfgCh:
		step.Command = fn
		step.HasCommand = true
		return step, true
	case <-timer.C:
		step.Timeout, step.TimedOut = wg.expireDecision()
		return step, true
	}
}

func (wg *waitGate[C]) expireDecision() (WaitTimeoutEvent, bool) {
	var event WaitTimeoutEvent

	if wg.timeout <= 0 {
		return event, false
	}
	now := wg.nowTime()

	wg.mu.Lock()
	defer wg.mu.Unlock()

	if wg.key == "" || wg.deadline.IsZero() || now.Before(wg.deadline) {
		return event, false
	}

	event.Key = wg.key
	event.Threshold = wg.timeout
	if !wg.since.IsZero() && now.After(wg.since) {
		event.Elapsed = now.Sub(wg.since)
	} else {
		event.Elapsed = wg.timeout
	}

	wg.clearLocked()
	return event, true
}

func (wg *waitGate[C]) currentKey() string {
	wg.mu.RLock()
	defer wg.mu.RUnlock()
	return wg.key
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
		wg.clearLocked()
	}
}

func (wg *waitGate[C]) clearLocked() {
	wg.key = ""
	wg.since = time.Time{}
	wg.deadline = time.Time{}
}

func (wg *waitGate[C]) nowTime() time.Time {
	if wg.now != nil {
		return wg.now()
	}
	return time.Now()
}

func (wg *waitGate[C]) setNow(now func() time.Time) {
	wg.mu.Lock()
	wg.now = now
	wg.mu.Unlock()
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
		configCommands:          opts.ConfigCommands,
		waitGate:                newWaitGate(opts.WaitKey, opts.WaitTimeout),
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
	h.waitGate.waitForDecision(cfg)
}

// WaitingForDecision reports whether config processing should currently wait
// for a matching enable/disable command.
func (h *Handler[C]) WaitingForDecision() bool {
	return h.waitGate.waitingForDecision()
}

// WaitDecisionRemaining returns time until wait gate timeout.
func (h *Handler[C]) WaitDecisionRemaining() (time.Duration, bool) {
	return h.waitGate.decisionRemaining()
}

// NextWaitDecisionStep blocks until either a dyncfg command arrives, wait timeout fires, or context is canceled.
// It centralizes wait-loop orchestration so caller logic stays minimal.
func (h *Handler[C]) NextWaitDecisionStep(ctx context.Context, dyncfgCh <-chan Function) (WaitDecisionStep, bool) {
	return h.waitGate.nextStep(ctx, dyncfgCh)
}

// ExpireWaitDecision clears the current wait gate when it exceeds configured timeout.
func (h *Handler[C]) ExpireWaitDecision() (WaitTimeoutEvent, bool) {
	return h.waitGate.expireDecision()
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

// StepRunner executes a command's blocking phase (effect) and then its
// commit. The synchronous runner executes both inline on the caller,
// preserving the legacy behavior; an asynchronous runner may execute the
// effect off-thread and the commit later, under this contract: commit runs
// EXACTLY ONCE after the effect returns, the two closures of one command
// never run concurrently, and a commit may invoke the runner again to chain
// another blocking phase of the same command.
type StepRunner func(effect func(context.Context) error, commit func(error))

// RunStepSync is the synchronous StepRunner used by the legacy Cmd* methods.
func RunStepSync(effect func(context.Context) error, commit func(error)) {
	commit(effect(context.Background()))
}

// ErrPhaseNeverRan is committed for a blocking phase that was never executed
// (runtime shutting down before it could run). Stop-shaped commits MUST NOT
// publish success on it: unlike a deadline abandonment - where the stop ran,
// wedged, and the job's output was fenced - nothing happened at all.
var ErrPhaseNeverRan = errors.New("the operation did not run: shutting down")

// ErrPhaseAbandoned is committed for a blocking phase that ran past its
// deadline (or into shutdown) and was abandoned with the job's output
// fenced. Stop-shaped commits publish success on it by contract - the
// stop is guaranteed to be effective even though the call has not
// returned. Any other stop error (e.g. a recovered panic) proves nothing
// about the job's state and MUST commit as failed.
var ErrPhaseAbandoned = errors.New("the operation was abandoned")

// CmdAddStep is CmdAdd with a caller-supplied StepRunner.
func (h *Handler[C]) CmdAddStep(fn Function, run StepRunner) { h.cmdAdd(fn, run) }

// CmdEnableStep is CmdEnable with a caller-supplied StepRunner.
func (h *Handler[C]) CmdEnableStep(fn Function, run StepRunner) { h.cmdEnable(fn, run) }

// CmdDisableStep is CmdDisable with a caller-supplied StepRunner.
func (h *Handler[C]) CmdDisableStep(fn Function, run StepRunner) { h.cmdDisable(fn, run) }

// CmdRemoveStep is CmdRemove with a caller-supplied StepRunner.
func (h *Handler[C]) CmdRemoveStep(fn Function, run StepRunner) { h.cmdRemove(fn, run) }

// CmdUpdateStep is CmdUpdate with a caller-supplied StepRunner.
func (h *Handler[C]) CmdUpdateStep(fn Function, run StepRunner) { h.cmdUpdate(fn, run) }

// CmdRestartStep is CmdRestart with a caller-supplied StepRunner.
func (h *Handler[C]) CmdRestartStep(fn Function, run StepRunner) { h.cmdRestart(fn, run) }

// CmdAdd handles the "add" command.
func (h *Handler[C]) CmdAdd(fn Function) {
	h.cmdAdd(fn, RunStepSync)
}

func (h *Handler[C]) cmdAdd(fn Function, run StepRunner) {
	if err := fn.ValidateArgs(3); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid config ID format.")
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	if err := h.cb.ValidateConfigName(name); err != nil {
		h.api.SendCodef(fn, 400, "invalid config name '%s': %v.", name, err)
		return
	}

	var newCfg C
	run(func(ctx context.Context) error {
		var err error
		newCfg, err = h.cb.ParseAndValidate(ctx, fn, name)
		return err
	}, func(err error) {
		if errors.Is(err, ErrPhaseNeverRan) {
			h.api.SendCodef(fn, 503, "%v", err)
			return
		}
		if err != nil {
			h.api.SendCodef(fn, 400, "%v", err)
			return
		}
		if h.cb.ConfigType(newCfg) != ConfigTypeJob {
			h.api.SendCodef(fn, 405, "adding configurations of type '%s' is not supported, only 'job' configurations can be added.", h.cb.ConfigType(newCfg))
			return
		}

		finish := func() {
			h.seen.Add(newCfg)
			newEntry := &Entry[C]{Cfg: newCfg, Status: StatusAccepted}
			h.exposed.Add(newEntry)

			h.api.SendCodef(fn, 202, "")
			h.NotifyConfigCreate(newCfg, StatusAccepted)
		}

		// Replace existing config at the same key, if any: cache removal
		// precedes the blocking stop (remove-before-block).
		if existing, ok := h.exposed.LookupByKey(key); ok {
			wasSeen := false
			if _, found := h.seen.Lookup(existing.Cfg); found && existing.Cfg.SourceType() == "dyncfg" {
				h.seen.Remove(existing.Cfg)
				wasSeen = true
			}
			existingEntry := existing
			h.exposed.Remove(existing.Cfg)
			run(func(ctx context.Context) error {
				h.cb.Stop(ctx, existing.Cfg)
				return nil
			}, func(stopErr error) {
				if errors.Is(stopErr, ErrPhaseNeverRan) {
					if wasSeen {
						h.seen.Add(existingEntry.Cfg)
					}
					h.exposed.Add(existingEntry)
					h.api.SendCodef(fn, 503, "%v", stopErr)
					return
				}
				if stopErr != nil && !errors.Is(stopErr, ErrPhaseAbandoned) {
					// The old instance's stop broke (recovered panic): do not
					// create a replacement next to a job in an unknown state.
					if wasSeen {
						h.seen.Add(existingEntry.Cfg)
					}
					h.exposed.Add(existingEntry)
					h.api.SendCodef(fn, 500, "%v", stopErr)
					return
				}
				finish()
			})
			return
		}
		finish()
	})
}

// CmdEnable handles the "enable" command.
func (h *Handler[C]) CmdEnable(fn Function) {
	h.cmdEnable(fn, RunStepSync)
}

func (h *Handler[C]) cmdEnable(fn Function, run StepRunner) {
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

	run(func(ctx context.Context) error {
		return h.cb.Start(ctx, entry.Cfg)
	}, func(err error) {
		if errors.Is(err, ErrPhaseNeverRan) {
			// The start never executed (shutdown): the config is untouched -
			// answer retryably without publishing a failed status (and
			// without the stock-removal side effect).
			h.api.SendCodef(fn, 503, "%v", err)
			return
		}
		if err != nil {
			entry.Status = StatusFailed

			code := h.enableFailCode
			if ce, ok2 := errors.AsType[CodedError](err); ok2 {
				code = ce.DyncfgCode()
			}
			h.api.SendCodef(fn, code, "%v", err)

			// Stock removal only for plain runtime detection failures.
			// CodedError carries an explicit status code, so the component owns the
			// failure semantics and whether the job should be retried.
			if h.removeStockOnEnableFail && !isCodedError(err) && entry.Cfg.SourceType() == "stock" {
				h.exposed.Remove(entry.Cfg)
				h.NotifyConfigRemove(entry.Cfg)
			} else {
				h.NotifyConfigStatus(entry.Cfg, StatusFailed)
			}

			h.cb.OnStatusChange(entry, oldStatus, fn)
			return
		}

		entry.Status = StatusRunning
		h.api.SendCodef(fn, 200, "%s", takeCommandMessage(h.cb))
		h.NotifyConfigStatus(entry.Cfg, StatusRunning)
		h.cb.OnStatusChange(entry, oldStatus, fn)
	})
}

// CmdDisable handles the "disable" command.
func (h *Handler[C]) CmdDisable(fn Function) {
	h.cmdDisable(fn, RunStepSync)
}

func (h *Handler[C]) cmdDisable(fn Function, run StepRunner) {
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
	run(func(ctx context.Context) error {
		h.cb.Stop(ctx, entry.Cfg)
		return nil
	}, func(stopErr error) {
		if errors.Is(stopErr, ErrPhaseNeverRan) {
			h.api.SendCodef(fn, 503, "%v", stopErr)
			return
		}
		if stopErr != nil && !errors.Is(stopErr, ErrPhaseAbandoned) {
			// The stop broke (recovered panic): the job's state is unknown
			// and nothing was fenced - publishing disabled would be a lie.
			entry.Status = StatusFailed
			h.api.SendCodef(fn, 500, "%v", stopErr)
			h.NotifyConfigStatus(entry.Cfg, StatusFailed)
			h.cb.OnStatusChange(entry, oldStatus, fn)
			return
		}
		entry.Status = StatusDisabled
		h.api.SendCodef(fn, 200, "%s", takeCommandMessage(h.cb))
		h.NotifyConfigStatus(entry.Cfg, StatusDisabled)
		h.cb.OnStatusChange(entry, oldStatus, fn)
	})
}

// CmdRemove handles the "remove" command.
func (h *Handler[C]) CmdRemove(fn Function) {
	h.cmdRemove(fn, RunStepSync)
}

func (h *Handler[C]) cmdRemove(fn Function, run StepRunner) {
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

	// Cache removal precedes the blocking stop (remove-before-block).
	h.seen.Remove(entry.Cfg)
	h.exposed.Remove(entry.Cfg)
	run(func(ctx context.Context) error {
		h.cb.Stop(ctx, entry.Cfg)
		return nil
	}, func(stopErr error) {
		if errors.Is(stopErr, ErrPhaseNeverRan) {
			// Nothing was stopped: restore the stage-time cache removals and
			// answer retryably instead of publishing a delete that is a lie.
			h.seen.Add(entry.Cfg)
			h.exposed.Add(entry)
			h.api.SendCodef(fn, 503, "%v", stopErr)
			return
		}
		if stopErr != nil && !errors.Is(stopErr, ErrPhaseAbandoned) {
			// The stop broke (recovered panic): restore the caches with a
			// failed status instead of publishing a delete for a job whose
			// state is unknown and whose output was never fenced.
			oldStatus := entry.Status
			entry.Status = StatusFailed
			h.seen.Add(entry.Cfg)
			h.exposed.Add(entry)
			h.api.SendCodef(fn, 500, "%v", stopErr)
			h.NotifyConfigStatus(entry.Cfg, StatusFailed)
			h.cb.OnStatusChange(entry, oldStatus, fn)
			return
		}
		h.api.SendCodef(fn, 200, "%s", takeCommandMessage(h.cb))
		h.NotifyConfigRemove(entry.Cfg)
	})
}

// CmdUpdate handles the "update" command.
func (h *Handler[C]) CmdUpdate(fn Function) {
	h.cmdUpdate(fn, RunStepSync)
}

func (h *Handler[C]) cmdUpdate(fn Function, run StepRunner) {
	key, name, ok := h.cb.ExtractKey(fn)
	if !ok {
		h.api.SendCodef(fn, 400, "invalid config ID format.")
		return
	}

	entry, ok := h.exposed.LookupByKey(key)
	if !ok {
		h.api.SendCodef(fn, 404, "config not found.")
		return
	}

	if err := fn.ValidateHasPayload(); err != nil {
		h.api.SendCodef(fn, 400, "%v", err)
		return
	}

	var newCfg C
	run(func(ctx context.Context) error {
		var err error
		newCfg, err = h.cb.ParseAndValidate(ctx, fn, name)
		return err
	}, func(err error) {
		if errors.Is(err, ErrPhaseNeverRan) {
			h.api.SendCodef(fn, 503, "%v", err)
			return
		}
		// Validation errors win over state rejections (400 beats 403): the
		// validation effect runs even for Accepted entries.
		if err != nil {
			h.api.SendCodef(fn, 400, "%v", err)
			h.NotifyConfigStatus(entry.Cfg, entry.Status)
			return
		}

		isConversion := entry.Cfg.SourceType() != "dyncfg"

		// No-op: running dyncfg config with same hash.
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

		proceed := func() {
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
					h.NotifyConfigCreate(newCfg, StatusDisabled)
				}
				h.api.SendCodef(fn, 200, "%s", takeCommandMessage(h.cb))
				h.NotifyConfigStatus(newCfg, StatusDisabled)
				h.cb.OnStatusChange(newEntry, oldStatus, fn)
				return
			}

			// Start or update.
			run(func(ctx context.Context) error {
				if isConversion {
					return h.cb.Start(ctx, newCfg)
				}
				return h.cb.Update(ctx, oldCfg, newCfg)
			}, func(err error) {
				if !isConversion && errors.Is(err, ErrPhaseNeverRan) {
					// The update phase never executed (shutdown): the old
					// instance is untouched - roll back to the old cache state
					// and answer retryably instead of publishing failed.
					h.seen.Remove(newCfg)
					h.seen.Add(oldCfg)
					h.exposed.Add(entry)
					h.api.SendCodef(fn, 503, "%v", err)
					h.NotifyConfigStatus(oldCfg, oldStatus)
					return
				}
				if err != nil {
					if !isConversion && errors.Is(err, ErrNonDisruptiveUpdate) {
						// Update failed before runtime disruption; rollback to old cache state.
						h.seen.Remove(newCfg)
						h.seen.Add(oldCfg)
						h.exposed.Add(entry)

						h.api.SendCodef(fn, 200, "%v", err)
						h.NotifyConfigStatus(oldCfg, oldStatus)
						// No OnStatusChange call here: effective state did not change.
						return
					}

					newEntry.Status = StatusFailed
					if isConversion {
						h.NotifyConfigCreate(newCfg, StatusFailed)
					}
					code := 200
					if ce, ok2 := errors.AsType[CodedError](err); ok2 {
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
				h.api.SendCodef(fn, 200, "%s", takeCommandMessage(h.cb))
				h.NotifyConfigStatus(newCfg, StatusRunning)
				h.cb.OnStatusChange(newEntry, oldStatus, fn)
			})
		}

		// For conversion, stop the old work before publishing replacement config state.
		if isConversion {
			run(func(ctx context.Context) error {
				h.cb.Stop(ctx, oldCfg)
				return nil
			}, func(stopErr error) {
				if errors.Is(stopErr, ErrPhaseNeverRan) {
					// Nothing happened at all: the old job is untouched and
					// the caches were not swapped yet - answer retryably
					// without marking anything failed.
					h.api.SendCodef(fn, 503, "%v", stopErr)
					return
				}
				// An abandoned or broken stop is a disruptive failure: the
				// update fails without cache rollback and the start phase
				// never begins.
				if stopErr != nil {
					entry.Status = StatusFailed
					code := 200
					if ce, ok2 := errors.AsType[CodedError](stopErr); ok2 {
						code = ce.DyncfgCode()
					}
					h.api.SendCodef(fn, code, "%v", stopErr)
					h.NotifyConfigStatus(oldCfg, StatusFailed)
					h.cb.OnStatusChange(entry, oldStatus, fn)
					return
				}
				proceed()
			})
			return
		}
		proceed()
	})
}

// CmdRestart handles the "restart" command.
// Stops the existing work and starts the same config again.
// Only allowed for Running/Failed configs (rejects Accepted/Disabled).
func (h *Handler[C]) CmdRestart(fn Function) {
	h.cmdRestart(fn, RunStepSync)
}

func (h *Handler[C]) cmdRestart(fn Function, run StepRunner) {
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

	switch entry.Status {
	case StatusAccepted, StatusDisabled:
		h.api.SendCodef(fn, 405, "restarting is not allowed in '%s' state.", entry.Status)
		h.NotifyConfigStatus(entry.Cfg, entry.Status)
		return
	case StatusRunning, StatusFailed:
		// proceed
	default:
		h.api.SendCodef(fn, 405, "restarting is not allowed in '%s' state.", entry.Status)
		h.NotifyConfigStatus(entry.Cfg, entry.Status)
		return
	}

	oldStatus := entry.Status

	run(func(ctx context.Context) error {
		h.cb.Stop(ctx, entry.Cfg)
		return nil
	}, func(stopErr error) {
		if errors.Is(stopErr, ErrPhaseNeverRan) {
			// Nothing happened at all: answer retryably without publishing a
			// failed status for a job that is in fact untouched.
			h.api.SendCodef(fn, 503, "%v", stopErr)
			return
		}
		// An abandoned stop (or a broken one) fails the restart and the
		// start phase never begins (no second instance).
		if stopErr != nil {
			entry.Status = StatusFailed
			code := 422
			if ce, ok2 := errors.AsType[CodedError](stopErr); ok2 {
				code = ce.DyncfgCode()
			}
			h.api.SendCodef(fn, code, "config restart failed: %v", stopErr)
			h.NotifyConfigStatus(entry.Cfg, StatusFailed)
			h.cb.OnStatusChange(entry, oldStatus, fn)
			return
		}
		run(func(ctx context.Context) error {
			return h.cb.Start(ctx, entry.Cfg)
		}, func(err error) {
			if err != nil {
				entry.Status = StatusFailed
				code := 422
				if ce, ok2 := errors.AsType[CodedError](err); ok2 {
					code = ce.DyncfgCode()
				}
				h.api.SendCodef(fn, code, "config restart failed: %v", err)
				h.NotifyConfigStatus(entry.Cfg, StatusFailed)
				h.cb.OnStatusChange(entry, oldStatus, fn)
				return
			}

			entry.Status = StatusRunning
			h.api.SendCodef(fn, 200, "")
			h.NotifyConfigStatus(entry.Cfg, StatusRunning)
			h.cb.OnStatusChange(entry, oldStatus, fn)
		})
	})
}

func isCodedError(err error) bool {
	var ce CodedError
	return errors.As(err, &ce)
}
