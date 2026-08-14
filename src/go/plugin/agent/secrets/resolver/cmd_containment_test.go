// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resolverResult struct {
	resolved any
	err      error
}

func TestDefaultAtomicResolverCancellationContainsCommandProcessGroup(t *testing.T) {
	if !resolverContainmentSupported() {
		t.Skip("process-group containment requires a Unix process model")
	}
	directory := t.TempDir()
	helper := filepath.Join(directory, "resolver-helper")
	pidFile := filepath.Join(directory, "resolver-pids")
	script := "#!/bin/sh\n" +
		"sleep 30 &\n" +
		"child=$!\n" +
		"printf '%s %s\\n' \"$$\" \"$child\" > \"$1\"\n" +
		"wait \"$child\"\n"
	require.NoError(t, os.WriteFile(helper, []byte(script), 0o700))

	resolver, err := NewDefaultAtomicResolver()
	require.NoError(t, err)
	input := map[string]any{
		"secret": "${cmd:" + helper + " " + pidFile + "}",
	}
	resolveCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan resolverResult, 1)
	go func() {
		resolved, err := resolver.Resolve(resolveCtx, input, nil)
		result <- resolverResult{resolved: resolved, err: err}
	}()

	var pidData []byte
	require.Eventually(t, func() bool {
		pidData, err = os.ReadFile(pidFile)
		return err == nil && len(pidData) != 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()
	var outcome resolverResult
	select {
	case outcome = <-result:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "resolver did not stop after cancellation")
	}
	require.Error(t, outcome.err)
	require.Nil(t, outcome.resolved)
	require.Equal(t, "${cmd:"+helper+" "+pidFile+"}", input["secret"])

	fields := strings.Fields(string(pidData))
	require.Len(t, fields, 2)
	pids := make([]int, len(fields))
	for index, field := range fields {
		pid, err := strconv.Atoi(field)
		require.NoError(t, err)
		require.Positive(t, pid)
		pids[index] = pid
	}
	require.Eventually(t, func() bool {
		for _, pid := range pids {
			if !resolverProcessGone(pid) {
				return false
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond)
}
