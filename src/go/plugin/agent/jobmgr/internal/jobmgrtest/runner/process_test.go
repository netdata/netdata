package runner

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessRoundTripAndRepeatableWait(t *testing.T) {
	requireShell(t)
	var output bytes.Buffer
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", `IFS= read -r line; printf '%s\n' "$line"; printf problem >&2`},
		ObserveOut: func(chunk []byte) error {
			_, err := output.Write(chunk)
			return err
		},
	})
	require.NoError(t, err)
	defer process.Kill()

	require.NoError(t, process.WriteContext(t.Context(), []byte("FUNCTION request\n")))
	first, firstErr := process.Wait(t.Context())
	require.NoError(t, firstErr)
	assert.Equal(t, "FUNCTION request\n", output.String())
	assert.Equal(t, "problem", string(first.Stderr))
	assert.False(t, first.StderrTruncated)

	second, secondErr := process.Wait(t.Context())
	require.NoError(t, secondErr)
	assert.Equal(t, first, second)
}

func TestProcessWaitCancellationKillsAndJoins(t *testing.T) {
	requireShell(t)
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", "sleep 30 & wait"},
	})
	require.NoError(t, err)
	defer process.Kill()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = process.Wait(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 2*time.Second)

	select {
	case <-process.Done():
	case <-time.After(time.Second):
		require.FailNow(t, "contained process did not join")
	}
	_, repeatedErr := process.Wait(t.Context())
	require.Error(t, repeatedErr)
	require.NotErrorIs(t, repeatedErr, context.DeadlineExceeded)
}

func TestProcessUsesSanitizedEnvironmentAndBoundsStderr(t *testing.T) {
	requireShell(t)
	t.Setenv("JOBMGR_ROOT_RUNNER_SECRET", "must-not-cross")
	var output bytes.Buffer
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments: []string{
			"-c",
			`printf '%s|%s' "$JOBMGR_ROOT_RUNNER_SECRET" "$LC_ALL"; head -c 70000 /dev/zero >&2`,
		},
		ObserveOut: func(chunk []byte) error {
			_, err := output.Write(chunk)
			return err
		},
	})
	require.NoError(t, err)
	defer process.Kill()

	result, err := process.Wait(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "|C", output.String())
	assert.Len(t, result.Stderr, stderrLimit)
	assert.True(t, result.StderrTruncated)
}

func TestProcessObserverFailureTerminatesChild(t *testing.T) {
	requireShell(t)
	want := errors.New("observer rejected output")
	process, err := Start(Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", "while :; do printf x; done"},
		ObserveOut: func([]byte) error {
			return want
		},
	})
	require.NoError(t, err)
	defer process.Kill()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = process.Wait(ctx)
	require.ErrorIs(t, err, want)
}

func TestProcessRejectsInvalidSpecification(t *testing.T) {
	tests := map[string]Spec{
		"relative executable": {Executable: "relative"},
		"relative directory":  {Executable: "/bin/cat", Directory: "relative"},
		"NUL executable":      {Executable: "/bin/cat\x00"},
		"NUL argument":        {Executable: "/bin/cat", Arguments: []string{"bad\x00argument"}},
	}
	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Start(spec)
			require.Error(t, err)
		})
	}
}

func requireShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
}
