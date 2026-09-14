package jobmgrtest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	assertFixtureCloseIsIdempotentAndJoins(
		t,
		"Agent",
		func(state *agentFixtureState) (fixtureCloseTarget, error) {
			fixture, err := startAgentFixtureWithState(context.Background(), false, state)
			if err != nil {
				return fixtureCloseTarget{}, err
			}
			return fixtureCloseTarget{
				state: fixture.state,
				close: fixture.close,
				done:  fixture.done,
			}, nil
		},
	)
}

func TestProcessFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	assertFixtureCloseIsIdempotentAndJoins(
		t,
		"Process",
		func(state *agentFixtureState) (fixtureCloseTarget, error) {
			fixture, err := startProcessFixture(context.Background(), state, time.Hour)
			if err != nil {
				return fixtureCloseTarget{}, err
			}
			return fixtureCloseTarget{
				state: fixture.state,
				close: fixture.close,
				done:  fixture.done,
			}, nil
		},
	)
}

type fixtureCloseTarget struct {
	state *agentFixtureState
	close func()
	done  <-chan struct{}
}

func assertFixtureCloseIsIdempotentAndJoins(
	t *testing.T,
	label string,
	start func(*agentFixtureState) (fixtureCloseTarget, error),
) {
	t.Helper()
	release := make(chan struct{})
	cleanupEntered := make(chan struct{})
	cleanupReturned := make(chan struct{})
	releaseCleanup := onceClose(release)
	t.Cleanup(releaseCleanup)

	fixture, err := start(&agentFixtureState{
		cleanupGate:     release,
		cleanupEntered:  cleanupEntered,
		cleanupReturned: cleanupReturned,
	})
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
		require.FailNowf(t, "test failed", "%s fixture did not enter held Cleanup", label)
	}
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNowf(t, "test failed", "%s fixture close was not bounded", label)
	}
	select {
	case <-cleanupReturned:
	case <-time.After(time.Second):
		releaseCleanup()
		require.FailNowf(t, "test failed", "%s fixture left held Cleanup behind", label)
	}
	fixture.close()
	select {
	case <-fixture.done:
	case <-time.After(time.Second):
		require.FailNowf(t, "test failed", "%s fixture did not join", label)
	}
}
