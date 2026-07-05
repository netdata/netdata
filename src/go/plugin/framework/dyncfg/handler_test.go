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

func (e *codedErr) Error() string   { return e.err.Error() }
func (e *codedErr) DyncfgCode() int { return e.code }

// mockCallbacks records all callback invocations for verification.
type mockCallbacks struct {
	extractKeyFn       func(fn Function) (string, string, bool)
	parsePayloadFn     func(fn Function, name string) error
	parseAndValidateFn func(fn Function, name string) (testConfig, error)
	startFn            func(cfg testConfig) error
	updateFn           func(oldCfg, newCfg testConfig) error
	stopFn             func(cfg testConfig)
	onStatusChangeFn   func(entry *Entry[testConfig], oldStatus Status, fn Function)
	configIDFn         func(cfg testConfig) string
	configTypeFn       func(cfg testConfig) ConfigType

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

func (m *mockCallbacks) ParsePayload(fn Function, name string) error {
	if m.parsePayloadFn != nil {
		return m.parsePayloadFn(fn, name)
	}
	return nil
}

func (m *mockCallbacks) ParseAndValidate(_ context.Context, fn Function, name string) (testConfig, error) {
	if m.parseAndValidateFn != nil {
		return m.parseAndValidateFn(fn, name)
	}
	return testConfig{uid: "dyncfg:" + name, key: name, sourceType: "dyncfg", source: "test"}, nil
}

func (m *mockCallbacks) ValidateConfigName(name string) error {
	return JobNameRuleStrict(name)
}

func (m *mockCallbacks) Start(_ context.Context, cfg testConfig) error {
	m.startCalls = append(m.startCalls, cfg)
	if m.startFn != nil {
		return m.startFn(cfg)
	}
	return nil
}

func (m *mockCallbacks) Update(_ context.Context, oldCfg, newCfg testConfig) error {
	m.updateCalls = append(m.updateCalls, updateCall{oldCfg, newCfg})
	if m.updateFn != nil {
		return m.updateFn(oldCfg, newCfg)
	}
	return nil
}

func (m *mockCallbacks) Stop(_ context.Context, cfg testConfig) {
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

func (m *mockCallbacks) ConfigType(cfg testConfig) ConfigType {
	if m.configTypeFn != nil {
		return m.configTypeFn(cfg)
	}
	return ConfigTypeJob
}

func newTestHandler(cb *mockCallbacks) *Handler[testConfig] {
	return newTestHandlerWithWaitTimeout(cb, 5*time.Second)
}

func newTestHandlerWithWaitTimeout(cb *mockCallbacks, waitTimeout time.Duration) *Handler[testConfig] {
	h, _ := newTestHandlerWithOutput(cb, waitTimeout)
	return h
}

func newTestHandlerWithOutput(cb *mockCallbacks, waitTimeout time.Duration) (*Handler[testConfig], *bytes.Buffer) {
	var buf bytes.Buffer
	api := NewResponder(netdataapi.New(safewriter.New(&buf)))
	h := NewHandler(HandlerOpts[testConfig]{
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
		ConfigCommands: []Command{
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
	return h, &buf
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

func TestCmdAdd_InvalidConfigName(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	cb.extractKeyFn = func(fn Function) (string, string, bool) {
		return "bad.name", "bad.name", true
	}

	fn := newTestFn("test:bad.name", "add", "bad.name", []byte(`{}`))
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.exposed.Count())
}

func TestCmdAdd_NonJobConfigTypeRejected(t *testing.T) {
	cb := &mockCallbacks{
		configTypeFn: func(testConfig) ConfigType { return ConfigTypeSingle },
	}
	h, out := newTestHandlerWithOutput(cb, 5*time.Second)

	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	h.CmdAdd(fn)

	assert.Equal(t, 0, h.seen.Count())
	assert.Equal(t, 0, h.exposed.Count())
	assert.Contains(t, out.String(), "405")
	assert.Contains(t, out.String(), "adding configurations of type 'single' is not supported, only 'job' configurations can be added.")
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

func TestCmdRemove_DyncfgSingleRejected(t *testing.T) {
	cb := &mockCallbacks{
		configTypeFn: func(testConfig) ConfigType { return ConfigTypeSingle },
	}
	h := newTestHandler(cb)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.seen.Add(cfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "remove", "", nil)
	h.CmdRemove(fn)

	_, ok := h.seen.LookupByUID("dyncfg:job1")
	assert.True(t, ok, "dyncfg single should not be removed")

	_, ok = h.exposed.LookupByKey("job1")
	assert.True(t, ok, "dyncfg single should remain exposed")
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

func TestCmdUpdate_ValidationPrecedesAcceptedStateRejection(t *testing.T) {
	tests := map[string]struct {
		parseErr string
	}{
		"invalid payload against accepted entry returns 400 not 403": {
			parseErr: "invalid payload before accepted rejection",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cb := &mockCallbacks{}
			h, out := newTestHandlerWithOutput(cb, 5*time.Second)

			oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
			h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusAccepted})

			cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
				return testConfig{}, errors.New(tc.parseErr)
			}

			fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
			h.CmdUpdate(fn)

			output := out.String()
			assert.Contains(t, output, `"status":400`)
			assert.Contains(t, output, tc.parseErr)
			assert.NotContains(t, output, `"status":403`)
			assert.Len(t, cb.updateCalls, 0)
			assert.Len(t, cb.startCalls, 0)

			entry, ok := h.exposed.LookupByKey("job1")
			require.True(t, ok)
			assert.Equal(t, StatusAccepted, entry.Status)
		})
	}
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

func TestCmdUpdate_NonConversion_StartFails_NonDisruptiveRollback(t *testing.T) {
	cb := &mockCallbacks{}
	cb.updateFn = func(_, _ testConfig) error {
		return MarkNonDisruptiveUpdate(errors.New("update preflight failed"))
	}
	h := newTestHandler(cb)

	oldCfg := testConfig{uid: "dyncfg:job1:v1", key: "job1", sourceType: "dyncfg", hash: 100}
	newCfg := testConfig{uid: "dyncfg:job1:v2", key: "job1", sourceType: "dyncfg", hash: 200}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
		return newCfg, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusRunning, entry.Status)
	assert.Equal(t, oldCfg.UID(), entry.Cfg.UID())

	_, ok := h.seen.LookupByUID(oldCfg.UID())
	assert.True(t, ok, "old config should be restored in seen cache")

	_, ok = h.seen.LookupByUID(newCfg.UID())
	assert.False(t, ok, "new config should be removed from seen cache on rollback")
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

func TestCmdUpdate_NonConversion_StartFails_CodedError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.updateFn = func(_, _ testConfig) error {
		return &codedErr{err: errors.New("bind failed"), code: 422}
	}
	h, out := newTestHandlerWithOutput(cb, 5*time.Second)

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
	assert.Contains(t, out.String(), `"status":422`)
	assert.Contains(t, out.String(), "bind failed")
}

func TestCmdUpdate_Conversion_StartFails_CodedError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error {
		return &codedErr{err: errors.New("bind failed"), code: 422}
	}
	h, out := newTestHandlerWithOutput(cb, 5*time.Second)

	oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock", hash: 100}
	h.seen.Add(oldCfg)
	h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})

	cb.parseAndValidateFn = func(_ Function, name string) (testConfig, error) {
		return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200, source: "test"}, nil
	}

	fn := newTestFn("test:job1", "update", "job1", []byte(`{}`))
	h.CmdUpdate(fn)

	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
	assert.Equal(t, "dyncfg", entry.Cfg.SourceType())
	assert.Contains(t, out.String(), `"status":422`)
	assert.Contains(t, out.String(), "bind failed")
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
	h, out := newTestHandlerWithOutput(cb, 5*time.Second)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	assert.Len(t, cb.stopCalls, 1)
	assert.Len(t, cb.startCalls, 1)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
	assert.Contains(t, out.String(), `"status":422`)
	assert.Contains(t, out.String(), "config restart failed: restart failed")
	require.Len(t, cb.statusCalls, 1)
	assert.Equal(t, StatusRunning, cb.statusCalls[0].oldStatus)
}

func TestCmdRestart_StartFails_CodedError(t *testing.T) {
	cb := &mockCallbacks{}
	cb.startFn = func(_ testConfig) error {
		return &codedErr{err: errors.New("bad config"), code: 400}
	}
	h, out := newTestHandlerWithOutput(cb, 5*time.Second)

	cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
	h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

	fn := newTestFn("test:job1", "restart", "", nil)
	h.CmdRestart(fn)

	entry, _ := h.exposed.LookupByKey("job1")
	assert.Equal(t, StatusFailed, entry.Status)
	assert.Contains(t, out.String(), `"status":400`)
	assert.Contains(t, out.String(), "config restart failed: bad config")
}

// --- Notify Tests ---

func TestNotifyConfigCreate_SupportedCommands(t *testing.T) {
	tests := []struct {
		name       string
		commands   []Command
		sourceType string
		configType ConfigType
		wantRemove bool
	}{
		{"dyncfg job with restart", []Command{CommandSchema, CommandGet, CommandRestart}, "dyncfg", ConfigTypeJob, true},
		{"dyncfg job no restart", []Command{CommandSchema, CommandGet}, "dyncfg", ConfigTypeJob, true},
		{"stock job with restart", []Command{CommandSchema, CommandGet, CommandRestart}, "stock", ConfigTypeJob, false},
		{"stock job no restart", []Command{CommandSchema, CommandGet}, "stock", ConfigTypeJob, false},
		{"dyncfg single no remove", []Command{CommandSchema, CommandGet, CommandUpdate}, "dyncfg", ConfigTypeSingle, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := &mockCallbacks{
				configTypeFn: func(testConfig) ConfigType { return tt.configType },
			}
			h := newTestHandler(cb)
			h.configCommands = tt.commands

			cfg := testConfig{sourceType: tt.sourceType}
			cmds := h.configSupportedCommands(cfg, tt.sourceType == "dyncfg")

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

// --- Job-name rule tests ---

func TestJobNameRuleStrict(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr bool
	}{
		"valid":              {input: "my_job"},
		"valid with numbers": {input: "job123"},
		"valid with dashes":  {input: "my-job"},
		"space":              {input: "my job", wantErr: true},
		"tab":                {input: "my\tjob", wantErr: true},
		"dot":                {input: "my.job", wantErr: true},
		"colon":              {input: "my:job", wantErr: true},
		"empty":              {input: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := JobNameRuleStrict(tt.input)
			if tt.wantErr {
				assert.Error(t, err, fmt.Sprintf("JobNameRuleStrict(%q) should fail", tt.input))
			} else {
				assert.NoError(t, err, fmt.Sprintf("JobNameRuleStrict(%q) should pass", tt.input))
			}
		})
	}
}

func TestJobNameRuleAllowDots(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr bool
	}{
		"valid":              {input: "my_job"},
		"valid with numbers": {input: "job123"},
		"valid with dashes":  {input: "my-job"},
		"dotted name":        {input: "my.job"},
		"fqdn":               {input: "host.example.com"},
		"space":              {input: "my job", wantErr: true},
		"tab":                {input: "my\tjob", wantErr: true},
		"colon":              {input: "my:job", wantErr: true},
		"empty":              {input: ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := JobNameRuleAllowDots(tt.input)
			if tt.wantErr {
				assert.Error(t, err, fmt.Sprintf("JobNameRuleAllowDots(%q) should fail", tt.input))
			} else {
				assert.NoError(t, err, fmt.Sprintf("JobNameRuleAllowDots(%q) should pass", tt.input))
			}
		})
	}
}

// --- Stop-commit outcome mapping ---

// commitErrRunner skips the blocking phase and commits err directly: the
// executor produces these outcomes (never-ran, abandoned, recovered panic)
// without the closure completing normally.
func commitErrRunner(err error) StepRunner {
	return func(_ func(context.Context) error, commit func(error)) {
		commit(err)
	}
}

// chainErrRunner executes the first phase normally (validation) and commits
// err for every later phase - the shape of a chained stop/update phase that
// the executor refused or that broke.
func chainErrRunner(err error) StepRunner {
	seq := 0
	return func(effect func(context.Context) error, commit func(error)) {
		if seq++; seq == 1 {
			commit(effect(context.Background()))
			return
		}
		commit(err)
	}
}

// TestStopCommitOutcomeMapping pins how stop-shaped commits map the three
// executor outcomes: abandoned (stop ran, wedged, output fenced) publishes
// success by contract; never-ran answers 503 and restores stage-time cache
// removals; anything else (a recovered panic) proves nothing about the job's
// state and must commit failed, never success.
func TestStopCommitOutcomeMapping(t *testing.T) {
	abandonedErr := fmt.Errorf("%w: timed out after 1s", ErrPhaseAbandoned)
	panicErr := errors.New("internal error: effect panic: boom")

	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"disable abandoned publishes disabled": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdDisableStep(newTestFn("test:job1", "disable", "", nil), commitErrRunner(abandonedErr))

				entry, _ := h.exposed.LookupByKey("job1")
				assert.Equal(t, StatusDisabled, entry.Status)
				assert.Contains(t, out.String(), `"status":200`)
				assert.Contains(t, out.String(), "status disabled")
			},
		},
		"disable never-ran answers 503 and publishes nothing": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdDisableStep(newTestFn("test:job1", "disable", "", nil), commitErrRunner(ErrPhaseNeverRan))

				entry, _ := h.exposed.LookupByKey("job1")
				assert.Equal(t, StatusRunning, entry.Status, "a stop that never ran must not change the status")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "status disabled")
			},
		},
		"disable broken stop commits failed, never disabled": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdDisableStep(newTestFn("test:job1", "disable", "", nil), commitErrRunner(panicErr))

				entry, _ := h.exposed.LookupByKey("job1")
				assert.Equal(t, StatusFailed, entry.Status)
				assert.Contains(t, out.String(), `"status":500`)
				assert.Contains(t, out.String(), "status failed")
				assert.NotContains(t, out.String(), "status disabled")
				require.Len(t, cb.statusCalls, 1)
			},
		},
		"remove abandoned publishes delete": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.seen.Add(cfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdRemoveStep(newTestFn("test:job1", "remove", "", nil), commitErrRunner(abandonedErr))

				_, exposed := h.exposed.LookupByKey("job1")
				assert.False(t, exposed, "the abandoned remove must commit the removal")
				assert.Contains(t, out.String(), `"status":200`)
				assert.Contains(t, out.String(), "delete")
			},
		},
		"remove never-ran restores caches and answers 503": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.seen.Add(cfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdRemoveStep(newTestFn("test:job1", "remove", "", nil), commitErrRunner(ErrPhaseNeverRan))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed, "the never-ran remove must restore the exposed entry")
				assert.Equal(t, StatusRunning, entry.Status)
				_, seen := h.seen.Lookup(cfg)
				assert.True(t, seen, "the never-ran remove must restore the seen entry")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "delete")
			},
		},
		"remove broken stop restores caches with failed status": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.seen.Add(cfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdRemoveStep(newTestFn("test:job1", "remove", "", nil), commitErrRunner(panicErr))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed, "the broken remove must restore the exposed entry")
				assert.Equal(t, StatusFailed, entry.Status)
				assert.Contains(t, out.String(), `"status":500`)
				assert.Contains(t, out.String(), "status failed")
				assert.NotContains(t, out.String(), "delete")
			},
		},
		"enable never-ran answers 503 without failing the entry": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				// Stock source: a failed enable would also trigger stock
				// removal - never-ran must trigger neither.
				cfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock"}
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusAccepted})

				h.CmdEnableStep(newTestFn("test:job1", "enable", "", nil), commitErrRunner(ErrPhaseNeverRan))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed, "never-ran must not remove the stock config")
				assert.Equal(t, StatusAccepted, entry.Status, "an unstarted config must not be marked failed")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "status failed")
			},
		},
		"conversion update stop never-ran answers 503 without failing the old job": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				oldCfg := testConfig{uid: "stock:job1", key: "job1", sourceType: "stock", hash: 100}
				h.seen.Add(oldCfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})
				cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
					return testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 200}, nil
				}

				h.CmdUpdateStep(newTestFn("test:job1", "update", "job1", []byte(`{}`)), chainErrRunner(ErrPhaseNeverRan))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed)
				assert.Equal(t, "stock:job1", entry.Cfg.UID(), "the old discovery entry must stay exposed")
				assert.Equal(t, StatusRunning, entry.Status, "an untouched old job must not be marked failed")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "status failed")
			},
		},
		"restart never-ran answers 503 without failing the entry": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				cfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg"}
				h.exposed.Add(&Entry[testConfig]{Cfg: cfg, Status: StatusRunning})

				h.CmdRestartStep(newTestFn("test:job1", "restart", "", nil), commitErrRunner(ErrPhaseNeverRan))

				entry, _ := h.exposed.LookupByKey("job1")
				assert.Equal(t, StatusRunning, entry.Status, "an untouched job must not be marked failed")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "status failed")
			},
		},
		"add-replace broken stop keeps the old instance": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
				h.seen.Add(oldCfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})
				cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
					return testConfig{uid: "dyncfg:job1-new", key: "job1", sourceType: "dyncfg", hash: 200}, nil
				}

				h.CmdAddStep(newTestFn("test:job1", "add", "job1", []byte(`{}`)), chainErrRunner(panicErr))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed, "the old entry must be restored")
				assert.Equal(t, "dyncfg:job1", entry.Cfg.UID(), "no replacement may be exposed next to a job in an unknown state")
				assert.Contains(t, out.String(), `"status":500`)
			},
		},
		"update chained never-ran rolls back and answers 503": {
			run: func(t *testing.T) {
				cb := &mockCallbacks{}
				h, out := newTestHandlerWithOutput(cb, 5*time.Second)
				oldCfg := testConfig{uid: "dyncfg:job1", key: "job1", sourceType: "dyncfg", hash: 100}
				h.seen.Add(oldCfg)
				h.exposed.Add(&Entry[testConfig]{Cfg: oldCfg, Status: StatusRunning})
				cb.parseAndValidateFn = func(_ Function, _ string) (testConfig, error) {
					return testConfig{uid: "dyncfg:job1-new", key: "job1", sourceType: "dyncfg", hash: 200}, nil
				}

				h.CmdUpdateStep(newTestFn("test:job1", "update", "job1", []byte(`{}`)), chainErrRunner(ErrPhaseNeverRan))

				entry, exposed := h.exposed.LookupByKey("job1")
				require.True(t, exposed)
				assert.Equal(t, "dyncfg:job1", entry.Cfg.UID(), "the never-ran update must roll back to the old config")
				assert.Equal(t, StatusRunning, entry.Status)
				_, seenOld := h.seen.Lookup(oldCfg)
				assert.True(t, seenOld, "the old seen entry must be restored")
				assert.Contains(t, out.String(), `"status":503`)
				assert.NotContains(t, out.String(), "status failed")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// TestHandler_CommandPlanForeignHoldStatusGates pins the status-axis scheduling
// class consumed by foreign-hold claim routing.
func TestHandler_CommandPlanForeignHoldStatusGates(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)

	tests := []struct {
		cmd      Command
		status   Status
		wantFree bool
		wantHeld bool
	}{
		{CommandEnable, StatusAccepted, true, true},
		{CommandEnable, StatusRunning, false, true},
		{CommandEnable, StatusFailed, true, true},
		{CommandEnable, StatusDisabled, true, true},
		{CommandRestart, StatusAccepted, false, true},
		{CommandRestart, StatusRunning, true, true},
		{CommandRestart, StatusFailed, true, true},
		{CommandRestart, StatusDisabled, false, true},
		{CommandDisable, StatusAccepted, true, true},
		{CommandDisable, StatusRunning, true, true},
		{CommandDisable, StatusFailed, true, true},
		{CommandDisable, StatusDisabled, false, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s %s", tc.cmd, tc.status), func(t *testing.T) {
			cfg := testConfig{uid: "uid-j", key: "j", sourceType: "dyncfg", source: "test"}
			entry := &Entry[testConfig]{Cfg: cfg, Status: tc.status}
			fn := newTestFn("test:j", string(tc.cmd), "", nil)

			plan := h.CommandPlan(fn, entry)
			assert.Equal(t, tc.wantFree, plan.NeedsClaims(false),
				"free lane intrinsic claim decision")
			assert.Equal(t, tc.wantHeld, plan.NeedsClaims(true),
				"foreign write hold must promote status-derived no-ops")
		})
	}

	for _, cmd := range []Command{CommandEnable, CommandRestart, CommandDisable} {
		t.Run(fmt.Sprintf("%s missing entry", cmd), func(t *testing.T) {
			fn := newTestFn("test:j", string(cmd), "", nil)
			plan := h.CommandPlan(fn, nil)
			assert.False(t, plan.NeedsClaims(false), "missing entry rejects claimless")
			assert.False(t, plan.NeedsClaims(true), "missing entry bypasses foreign holds")
		})
	}
}

// TestHandler_CommandPlanParity drives every state-gated command against
// every gate combination and asserts CommandPlan agrees with the Cmd*
// implementations: blocking work runs (the step runner is invoked) exactly
// when the plan reports intrinsic claims. The payload axis is part of the
// matrix: add and update answer 400 before any blocking work when the payload
// is missing.
func TestHandler_CommandPlanParity(t *testing.T) {
	type gate struct {
		sourceType string
		configType ConfigType
	}
	statuses := []Status{StatusAccepted, StatusRunning, StatusFailed, StatusDisabled}
	gates := []gate{
		{"dyncfg", ConfigTypeJob},
		{"user", ConfigTypeJob},
		{"stock", ConfigTypeJob},
		{"dyncfg", ConfigTypeSingle},
	}
	commands := []Command{CommandUpdate, CommandEnable, CommandRestart, CommandDisable, CommandRemove}
	payloads := []struct {
		name    string
		payload []byte
	}{
		{"with payload", []byte(`{}`)},
		{"without payload", nil},
		{"with unparsable payload", []byte("garbage")},
	}
	// The PayloadParser stage gate: "garbage" fails the cheap parse, so
	// add/update with it are rejection-only before any blocking work.
	parsePayload := func(fn Function, _ string) error {
		if string(fn.Fn().Payload) == "garbage" {
			return errors.New("unparsable payload")
		}
		return nil
	}

	driveWithSpy := func(h *Handler[testConfig], cmd Command, fn Function) bool {
		ran := false
		spy := func(effect func(context.Context) error, commit func(error)) {
			ran = true
			RunStepSync(effect, commit)
		}
		switch cmd {
		case CommandAdd:
			h.cmdAdd(fn, spy)
		case CommandUpdate:
			h.cmdUpdate(fn, spy)
		case CommandEnable:
			h.cmdEnable(fn, spy)
		case CommandRestart:
			h.cmdRestart(fn, spy)
		case CommandDisable:
			h.cmdDisable(fn, spy)
		case CommandRemove:
			h.cmdRemove(fn, spy)
		}
		return ran
	}

	for _, cmd := range commands {
		for _, st := range statuses {
			for _, g := range gates {
				for _, pl := range payloads {
					name := fmt.Sprintf("%s on %s %s %s %s", cmd, st, g.sourceType, g.configType, pl.name)
					t.Run(name, func(t *testing.T) {
						cb := &mockCallbacks{
							configTypeFn:   func(testConfig) ConfigType { return g.configType },
							parsePayloadFn: parsePayload,
						}
						h := newTestHandler(cb)
						cfg := testConfig{uid: "uid-j", key: "j", sourceType: g.sourceType, source: "test"}
						entry := &Entry[testConfig]{Cfg: cfg, Status: st}
						h.seen.Add(cfg)
						h.exposed.Add(entry)

						fn := newTestFn("test:j", string(cmd), "", pl.payload)

						// Capture the plan BEFORE running: the command's
						// commit mutates entry.Status (that is its job), and the
						// executor consults the plan pre-execution.
						plan := h.CommandPlan(fn, entry)
						claims := plan.NeedsClaims(false)
						if !claims && (cmd == CommandEnable || cmd == CommandRestart || cmd == CommandDisable) {
							assert.True(t, plan.NeedsClaims(true),
								"claimless status-derived gates must wait under foreign holds")
						} else {
							assert.Equal(t, claims, plan.NeedsClaims(true),
								"non-status gates must not change under foreign holds")
						}

						ran := driveWithSpy(h, cmd, fn)

						assert.Equal(t, claims, ran,
							"CommandPlan must mirror whether the handler runs blocking work")
					})
				}
			}
		}
	}

	// Add cells: a nil entry acts only for add, an existing entry takes the
	// replace path - both gated on payload presence AND the callback name
	// policy (the mock uses JobNameRuleStrict, so a dotted name answers 400
	// before any blocking work).
	for _, pl := range payloads {
		for _, existing := range []bool{false, true} {
			for _, jobName := range []string{"j", "bad.name"} {
				name := fmt.Sprintf("add %s existing=%v name=%s", pl.name, existing, jobName)
				t.Run(name, func(t *testing.T) {
					cb := &mockCallbacks{parsePayloadFn: parsePayload}
					h := newTestHandler(cb)
					var entry *Entry[testConfig]
					if existing {
						cfg := testConfig{uid: "uid-" + jobName, key: jobName, sourceType: "dyncfg", source: "test"}
						entry = &Entry[testConfig]{Cfg: cfg, Status: StatusRunning}
						h.seen.Add(cfg)
						h.exposed.Add(entry)
					}

					fn := newTestFn("test:"+jobName, string(CommandAdd), jobName, pl.payload)

					plan := h.CommandPlan(fn, entry)
					claims := plan.NeedsClaims(false)
					assert.Equal(t, claims, plan.NeedsClaims(true),
						"add gates are not status-derived")

					ran := driveWithSpy(h, CommandAdd, fn)

					assert.Equal(t, claims, ran,
						"CommandPlan must mirror whether the handler runs blocking work")
				})
			}
		}
	}
}
