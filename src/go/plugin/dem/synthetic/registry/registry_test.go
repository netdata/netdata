// SPDX-License-Identifier: GPL-3.0-or-later

package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// completingExecutor admits and completes every attempt with run.
type completingExecutor struct{ run synthetic.Run }

func (e completingExecutor) ExecuteScheduled(
	_ context.Context,
	_ synthetic.Request,
	_ func(synthetic.JobState),
	admitted func(time.Time) synthetic.Schedule,
) synthetic.Execution {
	admitted(time.Now())
	return synthetic.Execution{
		Drained: true,
		Run:     e.run,
	}
}

func newSession(t *testing.T, name string, executor monitor.Executor) *monitor.Session {
	t.Helper()
	session, err := monitor.New(synthetic.Request{
		Kind:    synthetic.Journey,
		Name:    name,
		Script:  "export {};",
		Timeout: time.Minute,
	}, time.Hour, executor)
	require.NoError(t, err)
	return session
}

func TestRegisterListsActiveSessionsUntilRetired(t *testing.T) {
	h := registry.New()
	assert.False(t, h.HasJobs())
	a, b := newSession(t, "a", completingExecutor{}), newSession(t, "b", completingExecutor{})
	retireB, err := h.Register(b, "https://example.org/")
	require.NoError(t, err)
	retireA, err := h.Register(a, "")
	require.NoError(t, err)
	_, err = h.Register(newSession(t, "a", completingExecutor{}), "")
	require.EqualError(t, err, "synthetic job already active")
	now := time.Now()
	assert.Equal(t, []registry.Job{
		{
			Snapshot: a.Snapshot(now),
		},
		{
			Target:   "https://example.org/",
			Snapshot: b.Snapshot(now),
		},
	}, h.Snapshot(now))
	assert.True(t, h.HasJobs())

	retireA()
	successor := newSession(t, "a", completingExecutor{})
	retireSuccessor, err := h.Register(successor, "")
	require.NoError(t, err)
	retireA()
	assert.Len(t, h.Snapshot(now), 2, "a retired session cannot remove its successor")
	retireSuccessor()
	retireB()
	assert.Empty(t, h.Snapshot(now))
	assert.False(t, h.HasJobs())
}

func TestLatestCopiesOneActiveRun(t *testing.T) {
	run := synthetic.Run{
		ID:          "r",
		Outcome:     synthetic.Failed,
		CompletedUS: time.Now().UnixMicro(),
		Events:      []synthetic.Event{{Message: "original"}},
	}
	h := registry.New()
	session := newSession(t, "checkout", completingExecutor{run: run})
	retire, err := h.Register(session, "")
	require.NoError(t, err)
	defer retire()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx) }()
	defer func() {
		cancel()
		require.NoError(t, <-done)
	}()
	require.Eventually(t, func() bool {
		_, ok := h.Latest("journey:checkout", "r")
		return ok
	}, 5*time.Second, time.Millisecond)
	got, _ := h.Latest("journey:checkout", "r")
	assert.Equal(t, run, got)
	got.Events[0].Message = "changed"
	again, _ := h.Latest("journey:checkout", "r")
	assert.Equal(t, "original", again.Events[0].Message)
	for name, key := range map[string][2]string{
		"other run":      {"journey:checkout", "other"},
		"inactive job":   {"journey:other", "r"},
		"empty identity": {"", ""},
	} {
		_, ok := h.Latest(key[0], key[1])
		assert.False(t, ok, name)
	}
}
