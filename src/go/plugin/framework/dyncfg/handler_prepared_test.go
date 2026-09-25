// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparedUpdate_AcceptErrorDisposesUnacceptedActivation(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)
	cfg := testConfig{
		uid:        "stock:job1",
		key:        "job1",
		sourceType: "stock",
		hash:       100,
	}
	incumbent := h.AddDiscoveredConfig(cfg, StatusRunning)
	acceptErr := errors.New("ownership unavailable")
	disposed := 0
	cb.prepareUpdateFn = func(Function, testConfig, testConfig) (PreparedActivation, error) {
		return &mockPreparedActivation{
			accept:  func() (func(), error) { return nil, acceptErr },
			dispose: func() { disposed++ },
		}, nil
	}

	prepared, err := h.Prepare(newTestFn("test:job1", "update", "", []byte(`{}`)))
	require.NoError(t, err)
	_, err = prepared.Apply(context.Background())
	require.ErrorIs(t, err, acceptErr)
	assert.Equal(t, 1, disposed)
	current, ok := h.exposed.LookupByKey("job1")
	require.True(t, ok)
	assert.Same(t, incumbent, current)
	assert.Empty(t, cb.stopCalls)
	assert.Empty(t, cb.statusCalls)
	_, ok = h.seen.Lookup(testConfig{
		uid: "dyncfg:job1",
	})
	assert.False(t, ok)
	require.Error(t, prepared.Dispose(context.Background()))
	assert.Equal(t, 1, disposed)
}

func TestPreparedCommand_AdoptionOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		command    string
		initial    Status
		enabled    bool
		source     string
		identical  bool
		code       int
		status     Status
		wantEnable bool
		preflight  int
	}{
		{"passive add", "add", StatusRunning, true, "dyncfg", false, 202, StatusAccepted, false, 0},
		{"enable passive", "enable", StatusAccepted, false, "dyncfg", false, 202, StatusAccepted, true, 0},
		{"enable pending", "enable", StatusAccepted, true, "dyncfg", false, 202, StatusAccepted, true, 0},
		{"enable running", "enable", StatusRunning, true, "dyncfg", false, 200, StatusRunning, true, 0},
		{"disabled update", "update", StatusDisabled, false, "dyncfg", false, 200, StatusDisabled, false, 0},
		{"pending update", "update", StatusAccepted, true, "dyncfg", false, 202, StatusAccepted, true, 1},
		{"running update", "update", StatusRunning, true, "dyncfg", false, 202, StatusAccepted, true, 1},
		{"failed update", "update", StatusFailed, true, "dyncfg", false, 202, StatusAccepted, true, 1},
		{"running identical update", "update", StatusRunning, true, "dyncfg", true, 200, StatusRunning, true, 0},
		{"running identical conversion", "update", StatusRunning, true, "stock", true, 202, StatusAccepted, true, 1},
		{"disabled conversion", "update", StatusDisabled, false, "stock", false, 200, StatusDisabled, false, 0},
		{"passive update rejected", "update", StatusAccepted, false, "dyncfg", false, 403, StatusAccepted, false, 0},
		{"disable pending", "disable", StatusAccepted, true, "dyncfg", false, 200, StatusDisabled, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cb := &mockCallbacks{}
			h, out := newTestHandlerWithOutput(cb)
			old := testConfig{
				uid:        tc.source + ":job1",
				key:        "job1",
				sourceType: tc.source,
				hash:       100,
			}
			incumbent := h.AddDiscoveredConfig(old, tc.initial)
			if tc.enabled && tc.initial == StatusAccepted {
				h.CmdEnable(newTestFn("test:job1", "enable", "", nil))
				incumbent, _ = h.exposed.LookupByKey("job1")
				out.Reset()
			}
			cb.parseAndValidateFn = func(Function, string) (testConfig, error) {
				hash := uint64(200)
				if tc.identical {
					hash = old.Hash()
				}
				return testConfig{
					uid:        "dyncfg:job1",
					key:        "job1",
					sourceType: "dyncfg",
					hash:       hash,
				}, nil
			}
			prepared, err := h.Prepare(newTestFn("test:job1", tc.command, "job1", []byte(`{}`)))
			require.NoError(t, err)
			current, _ := h.exposed.LookupByKey("job1")
			assert.Same(t, incumbent, current, "preparation must not change exposed state")
			assert.Empty(t, out.String(), "preparation must not publish")
			assert.Len(t, cb.updateCalls, tc.preflight)
			applied, err := prepared.Apply(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.code, applied.Result.Code)
			current, _ = h.exposed.LookupByKey("job1")
			assert.Equal(t, tc.status, current.Status)
			assert.Equal(t, tc.wantEnable, current.Enabled)
			assert.Empty(t, out.String(), "Apply returns owned frames without publishing")
			for _, notification := range applied.Notifications {
				require.NoError(t, notification.Validate())
				if notification.Kind == NotificationStatus {
					assert.Equal(t, current.Status, notification.Status)
				}
			}
		})
	}
}

func TestPreparedUpdate_UnacceptedOwnership(t *testing.T) {
	for _, mode := range []string{"dispose", "canceled apply", "stale predecessor"} {
		t.Run(mode, func(t *testing.T) {
			cb := &mockCallbacks{}
			h := newTestHandler(cb)
			cfg := testConfig{
				uid:        "dyncfg:job1",
				key:        "job1",
				sourceType: "dyncfg",
				hash:       100,
			}
			incumbent := h.AddDiscoveredConfig(cfg, StatusRunning)
			accepted, disposed := 0, 0
			cb.prepareUpdateFn = func(Function, testConfig, testConfig) (PreparedActivation, error) {
				return &mockPreparedActivation{
					accept:  func() (func(), error) { accepted++; return nil, nil },
					dispose: func() { disposed++ },
				}, nil
			}
			prepared, err := h.Prepare(newTestFn("test:job1", "update", "", []byte(`{}`)))
			require.NoError(t, err)
			switch mode {
			case "dispose":
				require.NoError(t, prepared.Dispose(context.Background()))
			case "canceled apply":
				ctx, cancel := context.WithCancelCause(context.Background())
				cause := errors.New("caller canceled before adoption")
				cancel(cause)
				_, err = prepared.Apply(ctx)
				require.ErrorIs(t, err, cause)
			case "stale predecessor":
				require.True(t, h.SetStatus(cfg, StatusFailed))
				incumbent, _ = h.exposed.LookupByKey("job1")
				applied, err := prepared.Apply(context.Background())
				require.NoError(t, err)
				assert.Equal(t, 409, applied.Result.Code)
				assert.Empty(t, applied.Notifications)
			}
			assert.Zero(t, accepted)
			assert.Equal(t, 1, disposed)
			current, _ := h.exposed.LookupByKey("job1")
			assert.Same(t, incumbent, current)
			assert.Empty(t, cb.stopCalls)
			assert.Empty(t, cb.statusCalls)
			require.Error(t, prepared.Dispose(context.Background()))
			_, err = prepared.Apply(context.Background())
			require.Error(t, err)
			assert.Equal(t, 1, disposed)
		})
	}
}

func TestPreparedAddPreservesUnrelatedOrUnacceptedDecision(t *testing.T) {
	for _, mode := range []string{"unrelated", "rejected", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			cb := &mockCallbacks{}
			h := newTestHandler(cb)
			waiting := testConfig{
				uid:        "file:job1",
				key:        "job1",
				sourceType: "user",
				source:     "file1",
			}
			h.AddDiscoveredConfig(waiting, StatusAccepted)
			h.AddDiscoveredConfig(
				testConfig{
					uid:        "file:job2",
					key:        "job2",
					sourceType: "user",
					source:     "file2",
				},
				StatusAccepted,
			)
			h.WaitForDecision(waiting)
			name := "job1"
			if mode == "unrelated" {
				name = "job2"
			}
			if mode == "rejected" {
				cb.parseAndValidateFn = func(Function, string) (testConfig, error) { return testConfig{}, errors.New("invalid config") }
			}
			prepared, err := h.Prepare(newTestFn("test:"+name, "add", name, []byte(`{}`)))
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			applied, err := prepared.Apply(ctx)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				want := 202
				if mode == "rejected" {
					want = 400
				}
				assert.Equal(t, want, applied.Result.Code)
			}
			assert.True(t, h.WaitingForDecision())
			h.SyncDecision(newTestFn("test:job1", "disable", "", nil))
			assert.False(t, h.WaitingForDecision(), "the original decision must still own the gate")
		})
	}
}

func TestPreparedUpdate_PreflightDoesNotHoldStateOwner(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)
	cfg := testConfig{
		uid:        "stock:job1",
		key:        "job1",
		sourceType: "stock",
		hash:       100,
	}
	h.AddDiscoveredConfig(cfg, StatusRunning)
	entered, release := make(chan struct{}), make(chan struct{})
	accepted, disposed := 0, 0
	cb.prepareUpdateFn = func(Function, testConfig, testConfig) (PreparedActivation, error) {
		close(entered)
		<-release
		return &mockPreparedActivation{
			accept:  func() (func(), error) { accepted++; return nil, nil },
			dispose: func() { disposed++ },
		}, nil
	}
	type preparation struct {
		command PreparedCommand
		err     error
	}
	done := make(chan preparation, 1)
	go func() {
		command, err := h.Prepare(newTestFn("test:job1", "update", "", []byte(`{}`)))
		done <- preparation{command, err}
	}()
	<-entered
	h.CmdDisable(newTestFn("test:job1", "disable", "", nil))
	incumbent, _ := h.exposed.LookupByKey("job1")
	require.Equal(t, StatusDisabled, incumbent.Status)
	close(release)
	prepared := <-done
	require.NoError(t, prepared.err)
	applied, err := prepared.command.Apply(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 409, applied.Result.Code)
	assert.Zero(t, accepted)
	assert.Equal(t, 1, disposed)
	current, _ := h.exposed.LookupByKey("job1")
	assert.Same(t, incumbent, current)
}

func TestPreparedCommand_PublicationReleasesActivation(t *testing.T) {
	for _, command := range []string{"enable", "update"} {
		t.Run(command, func(t *testing.T) {
			cb := &mockCallbacks{}
			h, out := newTestHandlerWithOutput(cb)
			cfg := testConfig{
				uid:        "dyncfg:job1",
				key:        "job1",
				sourceType: "dyncfg",
				hash:       100,
			}
			status := StatusDisabled
			if command == "update" {
				status = StatusRunning
			}
			h.AddDiscoveredConfig(cfg, status)
			activated, disposed := false, false
			published := func() {
				assert.Contains(t, out.String(), `"status":202`)
				assert.Contains(t, out.String(), "accepted")
				activated = true
			}
			cb.enableFn = func(testConfig) (func(), error) { return published, nil }
			cb.prepareUpdateFn = func(Function, testConfig, testConfig) (PreparedActivation, error) {
				return &mockPreparedActivation{
					accept:  func() (func(), error) { return published, nil },
					dispose: func() { disposed = true },
				}, nil
			}
			prepared, err := h.Prepare(newTestFn("test:job1", command, "", []byte(`{}`)))
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			applied, err := prepared.Apply(ctx)
			require.NoError(t, err)
			cancel() // Cancellation after adoption cannot change the accepted result.
			assert.False(t, activated)
			assert.False(t, disposed)
			require.Equal(t, 202, applied.Result.Code)
			h.api.output.FunctionResult(applied.Result)
			for _, notification := range applied.Notifications {
				notification.Emit(h.api.output)
			}
			require.NotNil(t, applied.Published)
			applied.Published()
			assert.True(t, activated)
		})
	}
}

func TestPreparedUpdate_RejectionPreservesExactIncumbent(t *testing.T) {
	for _, source := range []string{"stock", "dyncfg"} {
		t.Run(source, func(t *testing.T) {
			cb := &mockCallbacks{}
			h := newTestHandler(cb)
			cfg := testConfig{
				uid:        source + ":job1",
				key:        "job1",
				sourceType: source,
				hash:       100,
			}
			incumbent := h.AddDiscoveredConfig(cfg, StatusRunning)
			cb.updateFn = func(testConfig, testConfig) error { return errors.New("preflight failed") }
			prepared, err := h.Prepare(newTestFn("test:job1", "update", "", []byte(`{}`)))
			require.NoError(t, err)
			applied, err := prepared.Apply(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 422, applied.Result.Code)
			require.Len(t, applied.Notifications, 1)
			assert.Equal(t, StatusRunning, applied.Notifications[0].Status)
			current, _ := h.exposed.LookupByKey("job1")
			assert.Same(t, incumbent, current)
			seen, ok := h.seen.Lookup(cfg)
			require.True(t, ok)
			assert.Equal(t, cfg, seen)
			assert.Equal(t, 1, seenCacheCount(h.seen))
			assert.Empty(t, cb.stopCalls)
			assert.Empty(t, cb.statusCalls)
			assert.Nil(t, applied.Published)
		})
	}
}

func TestPreparedCommand_CanceledPreparationDoesNotParse(t *testing.T) {
	cb := &mockCallbacks{}
	cb.parseAndValidateFn = func(Function, string) (testConfig, error) {
		t.Fatal("canceled preparation must not parse input")
		return testConfig{}, nil
	}
	h := newTestHandler(cb)
	fn := newTestFn("test:job1", "add", "job1", []byte(`{}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fn.ctx = ctx
	prepared, err := h.Prepare(fn)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, prepared)
	assert.Zero(t, seenCacheCount(h.seen))
	assert.Zero(t, exposedCacheCount(h.exposed))
}

func TestPreparedAdd_ValidatesRawJobName(t *testing.T) {
	for _, name := range []string{"job one", "job:one", " job1", "job1 "} {
		t.Run(name, func(t *testing.T) {
			cb := &mockCallbacks{}
			cb.extractKeyFn = func(fn Function) (string, string, bool) {
				return fn.JobName(), fn.JobName(), true
			}
			cb.parseAndValidateFn = func(Function, string) (testConfig, error) {
				t.Fatal("invalid raw name must be rejected before payload parsing")
				return testConfig{}, nil
			}
			h := newTestHandler(cb)
			fn := newTestFn("test:template", "add", name, []byte(`{}`))
			assert.Equal(t, name, fn.JobName())
			prepared, err := h.Prepare(fn)
			require.NoError(t, err)
			applied, err := prepared.Apply(context.Background())
			require.NoError(t, err)
			assert.Equal(t, 400, applied.Result.Code)
			assert.Zero(t, seenCacheCount(h.seen))
			assert.Zero(t, exposedCacheCount(h.exposed))
		})
	}
}

func TestCmdEnable_PublishesAcceptanceBeforeContinuation(t *testing.T) {
	cb := &mockCallbacks{}
	h, out := newTestHandlerWithOutput(cb)
	cfg := testConfig{
		uid:        "stock:job1",
		key:        "job1",
		sourceType: "stock",
	}
	h.AddDiscoveredConfig(cfg, StatusAccepted)
	activated := false
	cb.enableFn = func(testConfig) (func(), error) {
		assert.Empty(t, out.String())
		return func() {
			assert.Contains(t, out.String(), `"status":202`)
			assert.Contains(t, out.String(), "accepted")
			activated = true
		}, nil
	}
	h.CmdEnable(newTestFn("test:job1", "enable", "", nil))
	assert.True(t, activated)
	// Repeating enable before activation settles must preserve the same intent.
	h.CmdEnable(newTestFn("test:job1", "enable", "", nil))
	assert.Len(t, cb.startCalls, 1)
}

func TestPreparedUpdate_DuplicateStatusKeepsPredecessor(t *testing.T) {
	cb := &mockCallbacks{}
	h := newTestHandler(cb)
	cfg := testConfig{
		uid:        "dyncfg:job1",
		key:        "job1",
		sourceType: "dyncfg",
		hash:       100,
	}
	incumbent := h.AddDiscoveredConfig(cfg, StatusRunning)
	prepared, err := h.Prepare(newTestFn("test:job1", "update", "", []byte(`{}`)))
	require.NoError(t, err)
	assert.False(t, h.SetStatus(cfg, StatusRunning), "duplicate status must not trigger publication")
	current, _ := h.exposed.LookupByKey("job1")
	assert.Same(t, incumbent, current)
	applied, err := prepared.Apply(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 202, applied.Result.Code)
}
