package jobmgrtest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	release := make(chan struct{})
	cleanupEntered := make(chan struct{})
	cleanupReturned := make(chan struct{})
	var releaseOnce sync.Once
	releaseCleanup := func() {
		releaseOnce.Do(func() { close(release) })
	}
	t.Cleanup(releaseCleanup)

	fixture, err := startAgentFixtureWithState(
		context.Background(),
		false,
		&agentFixtureState{
			cleanupGate:     release,
			cleanupEntered:  cleanupEntered,
			cleanupReturned: cleanupReturned,
		},
	)
	require.NoError(t, err)
	require.NoError(t, waitUntil(t.Context(), func() bool {
		return fixture.state.count("check") == 1
	}))

	closeDone := make(chan struct{})
	go func() {
		fixture.close()
		close(closeDone)
	}()
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Agent fixture did not enter held Cleanup")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Agent fixture close was not bounded")
	}
	select {
	case <-cleanupReturned:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Agent fixture left held Cleanup behind")
	}
	fixture.close()
	select {
	case <-fixture.done:
	case <-time.After(time.Second):
		require.FailNow(t, "Agent fixture did not join")
	}
}

func TestProcessFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	release := make(chan struct{})
	cleanupEntered := make(chan struct{})
	cleanupReturned := make(chan struct{})
	var releaseOnce sync.Once
	releaseCleanup := func() {
		releaseOnce.Do(func() { close(release) })
	}
	t.Cleanup(releaseCleanup)

	fixture, err := startProcessFixture(
		context.Background(),
		&agentFixtureState{
			cleanupGate:     release,
			cleanupEntered:  cleanupEntered,
			cleanupReturned: cleanupReturned,
		},
		time.Hour,
	)
	require.NoError(t, err)
	require.NoError(t, waitUntil(t.Context(), func() bool {
		return fixture.state.count("check") == 1
	}))

	closeDone := make(chan struct{})
	go func() {
		fixture.close()
		close(closeDone)
	}()
	select {
	case <-cleanupEntered:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Process fixture did not enter held Cleanup")
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Process fixture close was not bounded")
	}
	select {
	case <-cleanupReturned:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNow(t, "Process fixture left held Cleanup behind")
	}
	fixture.close()
	select {
	case <-fixture.done:
	case <-time.After(time.Second):
		require.FailNow(t, "Process fixture did not join")
	}
}
