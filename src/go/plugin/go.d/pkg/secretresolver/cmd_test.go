// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCmd_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	cfg := map[string]any{
		"password": "${cmd:/bin/echo hello}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "hello", cfg["password"])
}

func TestResolveCmd_TrimsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	// echo adds a trailing newline; TrimSpace removes it.
	cfg := map[string]any{
		"val": "${cmd:/bin/echo secretval}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "secretval", cfg["val"])
}

func TestResolveCmd_RelativePathRejected(t *testing.T) {
	cfg := map[string]any{
		"val": "${cmd:echo hello}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command path must be absolute")
}

func TestResolveCmd_EmptyCommand(t *testing.T) {
	cfg := map[string]any{
		"val": "${cmd:}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command")
}

func TestResolveCmd_NonexistentCommand(t *testing.T) {
	cfg := map[string]any{
		"val": "${cmd:/nonexistent/command}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
}

func TestResolveCmd_CommandWithArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	cfg := map[string]any{
		"val": "${cmd:/usr/bin/printf %s secret}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "secret", cfg["val"])
}

func TestResolveCmd_ScriptFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "secret.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho mysecret\n"), 0700))

	cfg := map[string]any{
		"val": "${cmd:" + script + "}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "mysecret", cfg["val"])
}
