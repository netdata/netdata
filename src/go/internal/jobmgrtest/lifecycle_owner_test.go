package jobmgrtest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	fixture, err := startAgentFixture(context.Background(), false)
	require.NoError(t, err)
	require.NoError(t, waitUntil(t.Context(), func() bool {
		return fixture.state.count("check") == 1
	}))

	fixture.close()
	fixture.close()
	select {
	case <-fixture.done:
	case <-time.After(time.Second):
		require.FailNow(t, "Agent fixture did not join")
	}
}

func TestProcessFixtureCloseIsIdempotentAndJoins(t *testing.T) {
	fixture, err := startProcessFixture(
		context.Background(),
		&agentFixtureState{},
		time.Second,
	)
	require.NoError(t, err)

	fixture.close()
	fixture.close()
	select {
	case <-fixture.done:
	case <-time.After(time.Second):
		require.FailNow(t, "Process fixture did not join")
	}
}
