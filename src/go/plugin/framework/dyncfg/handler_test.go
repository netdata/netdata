// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codedErr implements CodedError for testing.
type codedErr struct {
	err  error
	code int
}

func (e *codedErr) Error() string { return e.err.Error() }
func (e *codedErr) Code() int     { return e.code }

// mockCallbacks records all callback invocations for verification.
type mockCallbacks struct {
	extractKeyFn       func(fn Function) (string, string, bool)
	parseAndValidateFn func(fn Function, name string) (testConfig, error)
	startFn            func(cfg testConfig) error
	updateFn           func(oldCfg, newCfg testConfig) error
	stopFn             func(cfg testConfig)
	onStatusChangeFn   func(entry *Entry[testConfig], oldStatus Status, fn Function)
	configIDFn         func(cfg testConfig) string

	startCalls  []testConfig
	updateCalls []updateCall
	stopCalls   []testConfig
	statusCalls []statusChangeCall
}

type updateCall struct {
	oldCfg, newCfg testConfig
}

type statusChangeCall struct {
	entry     *Entry[testConfig]
	oldStatus Status
}

func (m *mockCallbacks) ExtractKey(fn Function) (string, string, bool) {
	if m.extractKeyFn != nil {
		return m.extractKeyFn(fn)
	}
	// Default: extract key from ID like "prefix:name".
	parts := strings.SplitN(fn.ID(), ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	return parts[1], parts[1], true
}

func (m *mockCallbacks) ParseAndValidate(fn Function, name string) (testConfig, error) {
	if m.parseAndValidateFn != nil {
		return m.parseAndValidateFn(fn, name)
	}
	return testConfig{uid: "dyncfg:" + name, key: name, sourceType: "dyncfg", source: "test"}, nil
}

func (m *mockCallbacks) Start(cfg testConfig) error {
	m.startCalls = append(m.startCalls, cfg)
	if m.startFn != nil {
		return m.startFn(cfg)
	}
	return nil
}

func (m *mockCallbacks) Update(oldCfg, newCfg testConfig) error {
	m.updateCalls = append(m.updateCalls, updateCall{oldCfg, newCfg})
	if m.updateFn != nil {
		return m.updateFn(oldCfg, newCfg)
	}
	return nil
}

func (m *mockCallbacks) Stop(cfg testConfig) {
	m.stopCalls = append(m.stopCalls, cfg)
	if m.stopFn != nil {
		m.stopFn(cfg)
	}
}

func (m *mockCallbacks) OnStatusChange(entry *Entry[testConfig], oldStatus Status, fn Function) {
	m.statusCalls = append(m.statusCalls, statusChangeCall{entry: entry, oldStatus: oldStatus})
	if m.onStatusChangeFn != nil {
		m.onStatusChangeFn(entry, oldStatus, fn)
	}
}

func (m *mockCallbacks) ConfigID(cfg testConfig) string {
	if m.configIDFn != nil {
		return m.configIDFn(cfg)
	}
	return "test:" + cfg.ExposedKey()
}

func newTestHandler(cb *mockCallbacks) *Handler[testConfig] {
	return newTestHandlerWithWaitTimeout(cb, 5*time.Second)
}

func newTestHandlerWithWaitTimeout(cb *mockCallbacks, waitTimeout time.Duration) *Handler[testConfig] {
	var buf bytes.Buffer
	api := NewResponder(netdataapi.New(safewriter.New(&buf)))
	return NewHandler(HandlerOpts[testConfig]{
		Logger:    logger.New(),
		API:       api,
		Seen:      NewSeenCache[testConfig](),
		Exposed:   NewExposedCache[testConfig](),
		Callbacks: cb,
		WaitKey: func(cfg testConfig) string {
			return cfg.Source()
		},
		WaitTimeout: waitTimeout,

		Path:                    "/test/path",
		EnableFailCode:          200,
		RemoveStockOnEnableFail: true,
		JobCommands: []Command{
			CommandSchema,
			CommandGet,
			CommandEnable,
			CommandDisable,
			CommandUpdate,
			CommandRestart,
			CommandTest,
			CommandUserconfig,
		},
	})
}

func newTestFn(id, cmd, name string, payload []byte) Function {
	args := []string{id, cmd}
	if name != "" {
		args = append(args, name)
	}
	return NewFunction(functions.Function{
		UID:     "test-uid",
		Args:    args,
		Payload: payload,
	})
}

func TestHandler_WaitForDecision_MatchingEnableClearsWait(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	h.WaitForDecision(cfg)
	assert.True(t, h.WaitingForDecision())

	h.SyncDecision(newTestFn("test:job1", "enable", "", nil))
	assert.False(t, h.WaitingForDecision())
}

func TestHandler_WaitForDecision_MismatchedCommandKeepsWait(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	waitCfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	otherCfg := testConfig{
		uid:        "uid-job2",
		key:        "job2",
		sourceType: "stock",
		source:     "mod/job2",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: waitCfg, Status: StatusAccepted})
	h.exposed.Add(&Entry[testConfig]{Cfg: otherCfg, Status: StatusAccepted})

	h.WaitForDecision(waitCfg)
	assert.True(t, h.WaitingForDecision())

	// Non enable/disable commands must not change wait state.
	h.SyncDecision(newTestFn("test:job1", "schema", "", nil))
	assert.True(t, h.WaitingForDecision())

	// Enable/disable for a different key must not clear wait state.
	h.SyncDecision(newTestFn("test:job2", "disable", "", nil))
	assert.True(t, h.WaitingForDecision())

	// Matching command clears wait state.
	h.SyncDecision(newTestFn("test:job1", "disable", "", nil))
	assert.False(t, h.WaitingForDecision())
}

func TestHandler_WaitForDecision_TimeoutClearsWait(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandlerWithWaitTimeout(cb, 5*time.Second)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	base := time.Unix(1000, 0)
	h.waitGate.setNow(func() time.Time { return base })

	h.WaitForDecision(cfg)
	assert.True(t, h.WaitingForDecision())

	h.waitGate.setNow(func() time.Time { return base.Add(4 * time.Second) })
	remaining, ok := h.WaitDecisionRemaining()
	assert.True(t, ok)
	assert.Equal(t, time.Second, remaining)

	_, timedOut := h.ExpireWaitDecision()
	assert.False(t, timedOut)
	assert.True(t, h.WaitingForDecision())

	h.waitGate.setNow(func() time.Time { return base.Add(5 * time.Second) })
	event, timedOut := h.ExpireWaitDecision()
	assert.True(t, timedOut)
	assert.Equal(t, "mod/job1", event.Key)
	assert.Equal(t, 5*time.Second, event.Threshold)
	assert.Equal(t, 5*time.Second, event.Elapsed)
	assert.False(t, h.WaitingForDecision())

	_, ok = h.WaitDecisionRemaining()
	assert.False(t, ok)
}

func TestHandler_WaitForDecision_TimeoutDisabledKeepsWait(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandlerWithWaitTimeout(cb, 0)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	base := time.Unix(1000, 0)
	h.waitGate.setNow(func() time.Time { return base })
	h.WaitForDecision(cfg)

	h.waitGate.setNow(func() time.Time { return base.Add(24 * time.Hour) })
	_, timedOut := h.ExpireWaitDecision()
	assert.False(t, timedOut)
	assert.True(t, h.WaitingForDecision())
}

func TestHandler_NextWaitDecisionStep_Command(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandlerWithWaitTimeout(cb, 5*time.Second)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})
	h.WaitForDecision(cfg)

	ch := make(chan Function, 1)
	fn := newTestFn("test:job1", "enable", "", nil)
	ch <- fn

	step, ok := h.NextWaitDecisionStep(context.Background(), ch)
	require.True(t, ok)
	require.True(t, step.HasCommand)
	assert.Equal(t, fn.UID(), step.Command.UID())
	assert.False(t, step.TimedOut)
}

func TestHandler_NextWaitDecisionStep_Timeout(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandlerWithWaitTimeout(cb, 20*time.Millisecond)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})
	h.WaitForDecision(cfg)

	ch := make(chan Function)
	step, ok := h.NextWaitDecisionStep(context.Background(), ch)
	require.True(t, ok)
	require.True(t, step.TimedOut)
	assert.Equal(t, "mod/job1", step.Timeout.Key)
	assert.False(t, h.WaitingForDecision())
}

func TestHandler_NextWaitDecisionStep_ContextCancel(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandlerWithWaitTimeout(cb, 5*time.Second)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "mod/job1",
	}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})
	h.WaitForDecision(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := make(chan Function)
	_, ok := h.NextWaitDecisionStep(ctx, ch)
	assert.False(t, ok)
	assert.True(t, h.WaitingForDecision())
}

func TestHandler_AddDiscoveredConfig_TracksSeenAndExposed(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{
		uid:        "uid-job1",
		key:        "job1",
		sourceType: "stock",
		source:     "file=/tmp/job1.conf",
	}

	h.RememberDiscoveredConfig(cfg)
	_, ok := h.seen.Lookup(cfg)
	require.True(t, ok, "config should be remembered in seen cache")

	entry := h.AddDiscoveredConfig(cfg, StatusAccepted)
	require.NotNil(t, entry)
	assert.Equal(t, StatusAccepted, entry.Status)
	assert.Equal(t, cfg.UID(), entry.Cfg.UID())

	exposed, ok := h.exposed.LookupByKey(cfg.ExposedKey())
	require.True(t, ok, "config should be exposed")
	assert.Equal(t, cfg.UID(), exposed.Cfg.UID())
	assert.Equal(t, StatusAccepted, exposed.Status)
}

func TestHandler_RemoveDiscoveredConfig_MismatchedExposedUID(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{
		uid:        "uid-stock",
		key:        "job1",
		sourceType: "stock",
		source:     "file=/tmp/job1.conf",
	}
	other := testConfig{
		uid:        "uid-dyncfg",
		key:        "job1",
		sourceType: "dyncfg",
		source:     "dyncfg=user",
	}

	h.seen.Add(cfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: other, Status: StatusRunning})

	entry, ok := h.RemoveDiscoveredConfig(cfg)
	require.False(t, ok, "mismatched exposed uid should not return an exposed entry")
	require.Nil(t, entry)

	_, stillSeen := h.seen.Lookup(cfg)
	assert.False(t, stillSeen, "seen config should be removed")
	exposed, stillExposed := h.exposed.LookupByKey(cfg.ExposedKey())
	require.True(t, stillExposed, "exposed entry with different uid should be preserved")
	assert.Equal(t, other.UID(), exposed.Cfg.UID())
}

// --- ExtractKey Failure Tests ---

func TestCmdAdd_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	// ID without ":" causes default ExtractKey to return false.
	fn := newTestFn("badid", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdEnable_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("badid", "enable", "", nil)
	h.CmdEnable(fn)

	assert.Len(t, cb.startCalls, 0)
}

func TestCmdDisable_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("badid", "disable", "", nil)
	h.CmdDisable(fn)

	assert.Len(t, cb.stopCalls, 0)
}

func TestCmdRemove_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("badid", "remove", "", nil)
	h.CmdRemove(fn)

	assert.Len(t, cb.stopCalls, 0)
}

func TestCmdUpdate_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("badid", "update", "", []byte(`{}`))
	h.CmdUpdate(fn)

	assert.Len(t, cb.updateCalls, 0)
}

func TestCmdRestart_ExtractKeyFailure(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("badid", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)
}

// --- CmdAdd Tests ---

func TestCmdAdd_Success(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	// Config should be in both caches.
	_, ok := h.seen.LookupByUID("dyncfg:job1")
	assert.True(t, ok, "config should be in seen cache")

	entry, ok := h.exposed.LookupByKey("job1")
	require.True(t, ok, "config should be in exposed cache")
	assert.Equal(t, StatusAccepted, entry.Status)
}

func TestCmdAdd_InvalidArgs(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	// Only 2 args (need 3).
	fn := newTestFn("test:job1", "add", "", nil)
	fn.fn.Args = fn.fn.Args[:2]
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdAdd_NoPayload(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "add", "job1", nil)
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdAdd_InvalidJobName(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cb.extractKeyFn = func(fn Function) (string, string, bool) {
		return "bad.name", "bad.name", true
	}

	fn := newTestFn("test:bad.name", "add", "bad.name", []byte(`{}`))
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdAdd_ParseError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{}, errors.New("bad config")
	}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdAdd_ReplacesExisting(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	// Old should be stopped, new should be in cache.
	require.Len(t, cb.stopCalls, 1)
	assert.Equal(t, "job1", cb.stopCalls[0].ExposedKey())

	entry, ok := h.exposed.LookupByKey("job1")
	require.True(t, ok)
	assert.Equal(t, StatusAccepted, entry.Status)
}

func TestCmdAdd_ReplacesExisting_KeepsNonDyncfgInSeen(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	// Existing is a stock config — should NOT be removed from seen.
	oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock"}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	// Stock config stays in seen (for re-promotion).
	_, ok := h.seen.LookupByUID("stock:job1")
	assert.True(t, ok, "stock config should remain in seen cache")
}

// --- CmdEnable Tests ---

func TestCmdEnable_Success(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	assert.Len(t, cb.startCalls, 1)
	assert.Len(t, cb.statusCalls, 1)
}

func TestCmdEnable_AlreadyRunning(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	// No Start called, no OnStatusChange.
	assert.Len(t, cb.startCalls, 0)
	assert.Len(t, cb.statusCalls, 0)
}

func TestCmdEnable_NotFound(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	assert.Len(t, cb.startCalls, 0)
}

func TestCmdEnable_StartFails_RegularError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error { return errors.New("start failed") }
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "stock"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	// Stock config should be removed on regular (non-coded) error.
	_, ok := h.exposed.LookupByKey("job1")
	assert.False(t, ok, "stock config should be removed from exposed on enable failure")
}

func TestCmdEnable_StartFails_CodedError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error {
		return &codedErr{err: errors.New("validation failed"), code: 400}
	}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "stock"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	// Stock config should NOT be removed on coded error.
	entry, ok := h.exposed.LookupByKey("job1")
	require.True(t, ok, "stock config should stay on coded error")
	assert.Equal(t, StatusFailed, entry.Status)
}

func TestCmdEnable_FromDisabled(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusDisabled})

	fn := newTestFn("test:job1", "enable", "", nil)
	h.CmdEnable(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusDisabled, cb.statusCalls[0].oldStatus)
}

// --- CmdDisable Tests ---

func TestCmdDisable_FromRunning(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "disable", "", nil)
	h.CmdDisable(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusDisabled, entry.Status)
	assert.Len(t, cb.stopCalls, 1)
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusRunning, cb.statusCalls[0].oldStatus)
}

func TestCmdDisable_AlreadyDisabled(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusDisabled})

	fn := newTestFn("test:job1", "disable", "", nil)
	h.CmdDisable(fn)

	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.statusCalls, 0)
}

func TestCmdDisable_FromFailed(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusFailed})

	fn := newTestFn("test:job1", "disable", "", nil)
	h.CmdDisable(fn)

	// Stop called unconditionally (may have retry tasks to cancel).
	assert.Len(t, cb.stopCalls, 1)
	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusDisabled, entry.Status)
}

func TestCmdDisable_NotFound(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "disable", "", nil)
	h.CmdDisable(fn)

	assert.Len(t, cb.stopCalls, 0)
}

// --- CmdRemove Tests ---

func TestCmdRemove_DyncfgConfig(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.seen.Add(cfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "remove", "", nil)
	h.CmdRemove(fn)

	_, ok := h.seen.LookupByUID("dyncfg:job1")
	assert.False(t, ok, "should be removed from seen")

	_, ok = h.exposed.LookupByKey("job1")
	assert.False(t, ok, "should be removed from exposed")

	assert.Len(t, cb.stopCalls, 1)
}

func TestCmdRemove_NonDyncfg_Rejected(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "remove", "", nil)
	h.CmdRemove(fn)

	// Should still be in cache — removal rejected.
	_, ok := h.exposed.LookupByKey("job1")
	assert.True(t, ok, "non-dyncfg config should not be removed")
	assert.Len(t, cb.stopCalls, 0)
}

func TestCmdRemove_NotFound(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "remove", "", nil)
	h.CmdRemove(fn)

	assert.Len(t, cb.stopCalls, 0)
}

// --- CmdUpdate Tests ---

func TestCmdUpdate_NonConversion_Success(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	// ParseAndValidate returns config with different hash.
	cb.parseAndValidateFn = func(_ Function, name string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200, source: "test"}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Should call Update (not Stop+Start).
	assert.Len(t, cb.updateCalls, 1)
	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)

	entry, ok := h.exposed.LookupByKey("job1")
	require.True(t, ok)
	assert.Equal(t, StatusRunning, entry.Status)
	assert.Equal(t, uint64(200), entry.Cfg.Hash())
}

func TestCmdUpdate_NonConversion_NoOp(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// No-op: same hash, running, not conversion.
	assert.Len(t, cb.updateCalls, 0)
	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)
}

func TestCmdUpdate_Conversion_Success(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, name string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200, source: "test"}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Conversion uses Stop + Start, not Update.
	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)
	assert.Len(t, cb.updateCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	assert.Equal(t, "dyncfg", entry.Cfg.SourceType())

	// Old stock config should still be in seen (for re-promotion).
	_, ok := h.seen.LookupByUID("stock:job1")
	assert.True(t, ok, "stock config should stay in seen for conversion")
}

func TestCmdUpdate_Disabled_PreservesStatus(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusDisabled})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Should NOT start, should preserve Disabled.
	assert.Len(t, cb.startCalls, 0)
	assert.Len(t, cb.updateCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusDisabled, entry.Status)
}

func TestCmdUpdate_Accepted_Rejected(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusAccepted})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Accepted configs can't be updated.
	assert.Len(t, cb.updateCalls, 0)
	assert.Len(t, cb.startCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusAccepted, entry.Status)
}

func TestCmdUpdate_NotFound(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	assert.Len(t, cb.updateCalls, 0)
}

func TestCmdUpdate_ParseError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{}, errors.New("bad config")
	}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Parse error should not modify cache.
	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
}

func TestCmdUpdate_NonConversion_StartFails(t *testing.T) {
	cb := &mockCallbacks{}
	cb.updateFn = func(_, _ testConfig) error { return errors.New("update failed") }
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
}

func TestCmdUpdate_Conversion_StartFails(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error { return errors.New("start failed") }
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, name string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200, source: "test"}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Conversion uses Stop + Start; Start fails → Failed status.
	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
	assert.Equal(t, "dyncfg", entry.Cfg.SourceType())
}

func TestCmdUpdate_NoPayload(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	// No payload (nil).
	fn := newTestFn("test:job1", "update", "job1", nil)
	h.CmdUpdate(fn)

	// Should fail with missing payload, not modify cache.
	assert.Len(t, cb.updateCalls, 0)
	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
}

func TestCmdUpdate_Conversion_Disabled(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock"}
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusDisabled})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", source: "test"}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	// Conversion with Disabled: Stop old, update caches, preserve Disabled.
	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusDisabled, entry.Status)
}

// --- CmdRestart Tests ---

func TestCmdRestart_FromRunning(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusRunning, cb.statusCalls[0].oldStatus)
}

func TestCmdRestart_FromFailed(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusFailed})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusFailed, cb.statusCalls[0].oldStatus)
}

func TestCmdRestart_Accepted_Rejected(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusAccepted, entry.Status)
}

func TestCmdRestart_Disabled_Rejected(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusDisabled})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusDisabled, entry.Status)
}

func TestCmdRestart_NotFound(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 0)
	assert.Len(t, cb.startCalls, 0)
}

func TestCmdRestart_StartFails(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error { return errors.New("restart failed") }
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusRunning, cb.statusCalls[0].oldStatus)
}

func TestCmdRestart_StartFails_CodedError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error {
		return &codedErr{err: errors.New("bad config"), code: 400}
	}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
}

// --- Notify Tests ---

func TestNotifyJobCreate_SupportedCommands(t *testing.T) {
	tests := []struct {
		name       string
		commands   []Command
		sourceType string
		wantRemove bool
	}{
		{"dyncfg with restart", []Command{CommandSchema, CommandGet, CommandRestart}, "dyncfg", true},
		{"dyncfg no restart", []Command{CommandSchema, CommandGet}, "dyncfg", true},
		{"stock with restart", []Command{CommandSchema, CommandGet, CommandRestart}, "stock", false},
		{"stock no restart", []Command{CommandSchema, CommandGet}, "stock", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &mockCallbacks{}
			h := newTestHandler(cb)
			h.jobCommands = tt.commands

			cmds := h.jobSupportedCommands(tt.sourceType == "dyncfg")

			// Base commands should always be present.
			for _, cmd := range tt.commands {
				assert.Contains(t, cmds, string(cmd))
			}
			if tt.wantRemove {
				assert.Contains(t, cmds, "remove")
			} else {
				assert.NotContains(t, cmds, "remove")
			}
		})
	}
}

// --- ValidateJobName Tests ---

func TestValidateJobName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "my_job", false},
		{"valid with numbers", "job123", false},
		{"valid with dashes", "my-job", false},
		{"space", "my job", true},
		{"tab", "my\tjob", true},
		{"dot", "my.job", true},
		{"colon", "my:job", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJobName(tt.input)
			if tt.wantErr {
				assert.Error(t, err, fmt.Sprintf("ValidateJobName(%q) should fail", tt.input))
			} else {
				assert.NoError(t, err, fmt.Sprintf("ValidateJobName(%q) should pass", tt.input))
			}
		})
	}
}
