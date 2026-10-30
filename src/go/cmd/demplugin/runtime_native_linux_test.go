// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNativeRuntimeRoleRouting(t *testing.T) {
	commands := []string{"native-prepare", "native-configure"}
	if os.Geteuid() == 0 {
		commands = []string{"native-initialize", "native-inspect", "native-qualify"}
	}
	for _, command := range commands {
		var output bytes.Buffer
		handled, err := runRuntimeCommand(context.Background(), []string{"runtime", command}, &output, &output)
		require.True(t, handled)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "unknown runtime command")
		require.Empty(t, output.String())
	}
}

func TestNativeChildEnvironmentDoesNotForwardInjection(t *testing.T) {
	identity := nativeIdentity{uid: 123, gid: 456, home: "/service home"}
	env := nativeServiceEnvironment(identity, []string{"HOME=/root", "USER=root", "LOGNAME=root", "PATH=/attacker", "LD_PRELOAD=synthetic-secret", "NODE_OPTIONS=synthetic-secret", "NETDATA_CLAIM_TOKEN=synthetic-secret", "HTTP_PROXY=http://secret.invalid", "CHROME_DEVEL_SANDBOX=/attacker/helper", "TMPDIR=/private tmp", "XDG_CACHE_HOME=/service/cache", "XDG_RUNTIME_DIR=relative", "TZ=:secret-file"})
	joined := strings.Join(env, "\n")
	require.NotContains(t, joined, "synthetic-secret")
	require.NotContains(t, joined, "attacker")
	require.NotContains(t, joined, "secret.invalid")
	require.Contains(t, env, "HOME=/service home")
	require.Contains(t, env, "USER=netdata")
	require.Contains(t, env, "LOGNAME=netdata")
	require.Contains(t, env, "TMPDIR=/private tmp")
	require.Contains(t, env, "XDG_CACHE_HOME=/service/cache")
	require.NotContains(t, joined, "XDG_RUNTIME_DIR")
	require.NotContains(t, joined, "TZ=")
}

// This reader has no WriterTo optimization, so io.Copy also verifies that the
// bounded output cannot accidentally inherit bytes.Buffer.ReadFrom.
type nativeTestReader struct{ io.Reader }

func TestNativeChildOutputRemainsBoundedWhenDrained(t *testing.T) {
	output := nativeOutput{limit: 64}
	written, err := io.Copy(&output, nativeTestReader{strings.NewReader(strings.Repeat("x", 4096))})
	require.NoError(t, err)
	require.EqualValues(t, 4096, written)
	require.Len(t, output.Bytes(), 64)
	require.True(t, output.overflow)
	n, err := output.Write([]byte("more"))
	require.NoError(t, err)
	require.Equal(t, 4, n)
	require.Len(t, output.Bytes(), 64)
}

func TestNativeCredentialChild(t *testing.T) {
	separator := slices.Index(os.Args, "--native-credential-fixture")
	if separator < 0 {
		return
	}
	mode := os.Args[separator+1]
	if mode == "fail" {
		os.Exit(23)
	}
	groups, err := os.Getgroups()
	if err != nil {
		os.Exit(2)
	}
	result := struct {
		UID    int      `json:"uid"`
		GID    int      `json:"gid"`
		Groups []int    `json:"groups"`
		Env    []string `json:"env"`
		Args   []string `json:"args"`
	}{os.Geteuid(), os.Getegid(), groups, os.Environ(), os.Args[separator+2:]}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestNativeChildDropsActualCredentialsAndPreservesArguments(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("credential drop test requires isolated root fixture")
	}
	identity, err := nativeServiceAccount()
	if err != nil {
		t.Skip("credential drop test requires netdata account")
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	if err := trustedNativeExecutable(executable); err != nil {
		t.Skip("credential fixture binary must be mounted at a trusted root-owned path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Setenv("NODE_OPTIONS", "synthetic-secret")
	t.Setenv("NETDATA_CLAIM_TOKEN", "synthetic-secret")
	arguments := []string{"space value", "$(not a command)", "a;b", "single'quote"}
	args := append([]string{"-test.run=^TestNativeCredentialChild$", "--", "--native-credential-fixture", "identity"}, arguments...)
	command := nativeCredentialCommand(ctx, executable, args, identity)
	data, err := command.Output()
	require.NoError(t, err)
	var result struct {
		UID, GID  int
		Groups    []int
		Env, Args []string
	}
	require.NoError(t, json.Unmarshal(data, &result))
	require.EqualValues(t, identity.uid, result.UID)
	require.EqualValues(t, identity.gid, result.GID)
	for _, group := range identity.groups {
		require.Contains(t, result.Groups, int(group))
	}
	require.Equal(t, arguments, result.Args)
	require.NotContains(t, strings.Join(result.Env, "\n"), "synthetic-secret")
	require.Contains(t, result.Env, "HOME="+identity.home)
	command = nativeCredentialCommand(ctx, executable, []string{"-test.run=^TestNativeCredentialChild$", "--", "--native-credential-fixture", "fail"}, identity)
	err = command.Run()
	var exit *exec.ExitError
	require.True(t, errors.As(err, &exit))
	require.Equal(t, 23, exit.ExitCode())
	require.NotEqual(t, "0", strconv.Itoa(result.UID))
}

type nativeFailingDiagnostic struct{ err error }

func (w nativeFailingDiagnostic) Write([]byte) (int, error) { return 0, w.err }

func TestNativeChildRejectsBeforeExecution(t *testing.T) {
	failure := errors.New("diagnostic writer failed")
	child := nativeChild{executable: "/must-not-execute", identity: nativeIdentity{uid: 123, gid: 456}, diagnostic: nativeFailingDiagnostic{failure}}
	_, err := child.run(context.Background(), "unexpected", nil)
	require.ErrorContains(t, err, "invalid native service")
	_, err = child.run(context.Background(), "native-inspect", make([]byte, nativeProtocolLimit+1))
	require.ErrorContains(t, err, "input exceeds")
	_, err = child.run(context.Background(), "native-inspect", nil)
	require.ErrorIs(t, err, failure, "logging failure must precede any command execution")
	child.identity.uid = 0
	_, err = child.run(context.Background(), "native-inspect", nil)
	require.ErrorContains(t, err, "must not run as root")
}
