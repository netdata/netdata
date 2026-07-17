// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fakeAgentEnabledEnv = "NETDATA_FUNCTION_CALL_FAKE_AGENT"
	fakeAgentModeEnv    = "NETDATA_FUNCTION_CALL_FAKE_AGENT_MODE"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeAgentEnabledEnv) == "1" {
		os.Exit(runFakeAgent(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

func TestCallAgentProcessLifecycle(t *testing.T) {
	t.Run("publication result and clean QUIT", func(t *testing.T) {
		result, err := callAgent(context.Background(), fakeAgentConfig(t, "success"))
		require.NoError(t, err)
		assert.Equal(t, 200, result.status)
		assert.JSONEq(t, `{"status":200}`, string(result.payload))
	})

	t.Run("rounded protocol timeout permits delayed result", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "delayed-result")
		cfg.functionTimeout = 20 * time.Millisecond

		result, err := callAgent(context.Background(), cfg)
		require.NoError(t, err)
		assert.Equal(t, 200, result.status)
	})

	t.Run("publication timeout cancels and reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "no-publication")
		cfg.startupTimeout = 20 * time.Millisecond

		started := time.Now()
		_, err := callAgent(context.Background(), cfg)
		assert.ErrorIs(t, err, errFunctionNotPublished)
		assert.Less(t, time.Since(started), time.Second)
	})

	t.Run("exit before publication is observed", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "exit-before-publication")

		_, err := callAgent(context.Background(), cfg)
		assert.ErrorContains(t, err, "before publication")
	})

	t.Run("result timeout cancels and reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "no-result")
		cfg.functionTimeout = time.Nanosecond

		started := time.Now()
		_, err := callAgent(context.Background(), cfg)
		assert.ErrorContains(t, err, "Function result timed out")
		assert.Less(t, time.Since(started), 3*time.Second)
	})

	t.Run("exit before result is observed", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "exit-before-result")

		_, err := callAgent(context.Background(), cfg)
		assert.ErrorContains(t, err, "before result")
	})

	t.Run("malformed result is observed", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "malformed-result")

		_, err := callAgent(context.Background(), cfg)
		assert.ErrorContains(t, err, "malformed FUNCTION_RESULT_BEGIN")
	})

	t.Run("shutdown timeout forces termination and reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "ignore-quit")
		cfg.shutdownTimeout = 20 * time.Millisecond

		started := time.Now()
		_, err := callAgent(context.Background(), cfg)
		assert.ErrorContains(t, err, "Agent shutdown timed out")
		assert.Less(t, time.Since(started), time.Second)
	})

	t.Run("caller cancellation during publication reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "no-publication")
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)

		started := time.Now()
		_, err := callAgent(ctx, cfg)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(started), time.Second)
	})

	t.Run("caller cancellation during result reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "no-result")
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(20*time.Millisecond, cancel)

		started := time.Now()
		_, err := callAgent(ctx, cfg)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(started), time.Second)
	})

	t.Run("caller cancellation during shutdown reaps", func(t *testing.T) {
		cfg := fakeAgentConfig(t, "ignore-quit")
		cfg.shutdownTimeout = time.Second
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)

		started := time.Now()
		_, err := callAgent(ctx, cfg)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(started), time.Second)
	})
}

func fakeAgentConfig(t *testing.T, mode string) callConfig {
	t.Helper()
	t.Setenv(fakeAgentEnabledEnv, "1")
	t.Setenv(fakeAgentModeEnv, mode)
	return callConfig{
		pluginPath:      os.Args[0],
		configDir:       t.TempDir(),
		module:          "test",
		function:        "test:echo",
		startupTimeout:  200 * time.Millisecond,
		functionTimeout: 200 * time.Millisecond,
		shutdownTimeout: 2 * time.Second,
		stderr:          io.Discard,
	}
}

func runFakeAgent(stdin io.Reader, stdout io.Writer) int {
	mode := os.Getenv(fakeAgentModeEnv)
	if mode == "exit-before-publication" {
		return 7
	}
	if mode == "no-publication" {
		_, _ = io.Copy(io.Discard, stdin)
		return 0
	}

	writer := bufio.NewWriter(stdout)
	_, _ = fmt.Fprintln(writer,
		`FUNCTION GLOBAL "test:echo" 60 "help" "test" 0xFFFF 1 1`)
	if writer.Flush() != nil {
		return 2
	}

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return 3
	}
	reader := csv.NewReader(strings.NewReader(scanner.Text()))
	reader.Comma = ' '
	fields, err := reader.Read()
	if err != nil || len(fields) != 6 ||
		fields[0] != "FUNCTION" ||
		fields[1] != functionUID ||
		fields[3] != "test:echo" {
		return 4
	}
	if mode == "no-result" {
		_, _ = io.Copy(io.Discard, stdin)
		return 0
	}
	if mode == "exit-before-result" {
		return 8
	}
	if mode == "delayed-result" {
		time.Sleep(75 * time.Millisecond)
	}
	if mode == "malformed-result" {
		_, _ = fmt.Fprintln(writer, "FUNCTION_RESULT_BEGIN malformed")
		_ = writer.Flush()
		return 9
	}

	_, _ = fmt.Fprintln(writer,
		"FUNCTION_RESULT_BEGIN functions-validation 200 application/json 1")
	_, _ = fmt.Fprintln(writer, `{"status":200}`)
	_, _ = fmt.Fprintln(writer, "FUNCTION_RESULT_END")
	if writer.Flush() != nil {
		return 5
	}

	for scanner.Scan() {
		if scanner.Text() != "QUIT" {
			continue
		}
		if mode == "ignore-quit" {
			time.Sleep(10 * time.Second)
		}
		return 0
	}
	return 6
}
