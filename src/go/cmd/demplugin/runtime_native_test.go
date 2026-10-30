// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

func TestNativeRuntimeDispatchRejectsOverrides(t *testing.T) {
	for _, args := range [][]string{nil, {"runtime"}, {"runtime", "status"}, {"--version"}} {
		var output bytes.Buffer
		handled, err := runNativeRuntimeCommand(context.Background(), args, &output, &output)
		require.False(t, handled)
		require.NoError(t, err)
		require.Empty(t, output.String())
	}
	for _, args := range [][]string{{"runtime", "native-unknown"}, {"runtime", "native-prepare", "--root", "/other"}, {"runtime", "native-configure", "--service", "root"}, {"runtime", "native-inspect", "stray"}, {"runtime", "native-qualify", "--no-sandbox"}} {
		var output bytes.Buffer
		handled, err := runNativeRuntimeCommand(context.Background(), args, &output, &output)
		require.True(t, handled)
		require.Error(t, err)
		require.Empty(t, output.String())
	}
}

func TestNativeProtocolIsBoundedAndStrict(t *testing.T) {
	for _, data := range []string{"", "{", "{} {}", "{\"unknown\":1}", strings.Repeat(" ", nativeProtocolLimit+1)} {
		var target runtime.Manifest
		require.Error(t, decodeNativeProtocol(strings.NewReader(data), &target))
	}
	for _, data := range []string{"null", "{}", "{\"process_claims\":0}", "{\"process_claims\":null,\"candidate_claims\":0,\"execution_pins\":0,\"uncertain_pins\":0}", "{\"process_claims\":0,\"candidate_claims\":0,\"execution_pins\":0,\"uncertain_pins\":0,\"extra\":1}"} {
		_, err := decodeNativeStatus([]byte(data))
		require.Error(t, err)
	}
	data, err := json.Marshal(runtime.NativeStatus{ProcessClaims: 1, UncertainPins: 2, ExecutionPins: 2})
	require.NoError(t, err)
	status, err := decodeNativeStatus(data)
	require.NoError(t, err)
	require.Equal(t, 1, status.ProcessClaims)
	require.Equal(t, 2, status.UncertainPins)
}

func TestNativeQualificationRequiresEveryProofAndExactIdentity(t *testing.T) {
	target := runtime.Manifest{Version: "fixture", Contract: 1, Archive: runtime.Archive{SHA256: strings.Repeat("a", 64)}}
	qualified := runner.Qualification{ExecutionContract: true, SandboxedLocalJourney: true, SandboxedLocalLighthouse: true, ProcessTreeDrained: true, Runtime: &synthetic.RuntimeIdentity{Version: target.Version, Contract: target.Contract, SHA256: target.Archive.SHA256}}
	encode := func(value runner.Qualification) []byte {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return data
	}
	require.NoError(t, verifyNativeQualification(encode(qualified), target))
	for _, change := range []func(*runner.Qualification){
		func(q *runner.Qualification) { q.ExecutionContract = false },
		func(q *runner.Qualification) { q.SandboxedLocalJourney = false },
		func(q *runner.Qualification) { q.SandboxedLocalLighthouse = false },
		func(q *runner.Qualification) { q.ProcessTreeDrained = false },
		func(q *runner.Qualification) { q.Runtime = nil },
		func(q *runner.Qualification) {
			q.Runtime = &synthetic.RuntimeIdentity{Version: "different", Contract: 1, SHA256: target.Archive.SHA256}
		},
	} {
		value := qualified
		change(&value)
		require.Error(t, verifyNativeQualification(encode(value), target))
	}
	require.Error(t, verifyNativeQualification([]byte("{}"), target))
}
