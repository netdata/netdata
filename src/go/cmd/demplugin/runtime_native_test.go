// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/stretchr/testify/require"
)

func TestNativeRuntimeDispatchRejectsOverrides(t *testing.T) {
	for _, args := range [][]string{{"native-unknown"}, {"native-prepare", "--root", "/other"}, {"native-configure", "--service", "root"}, {"native-inspect", "stray"}, {"native-validate", "--no-sandbox"}, {"native-status", "--root", "/other"}} {
		var output bytes.Buffer
		err := runRuntimeCommand(context.Background(), args, nil, &output, &output)
		require.Error(t, err)
		require.Empty(t, output.String())
	}
}

func TestNativeProtocolIsBoundedAndStrict(t *testing.T) {
	for _, data := range []string{"", "{", "{} {}", "{\"unknown\":1}", strings.Repeat(" ", nativeProtocolLimit+1)} {
		var target browserruntime.Manifest
		require.Error(t, decodeNativeProtocol(strings.NewReader(data), &target))
	}
	for _, data := range []string{"null", "{}", "{\"process_claims\":0}", "{\"process_claims\":null,\"candidate_claims\":0,\"execution_pins\":0,\"uncertain_pins\":0}", "{\"process_claims\":0,\"candidate_claims\":0,\"execution_pins\":0,\"uncertain_pins\":0,\"extra\":1}"} {
		_, err := decodeNativeStatus([]byte(data))
		require.Error(t, err)
	}
	data, err := json.Marshal(browserruntime.NativeStatus{
		ProcessClaims: 1,
		UncertainPins: 2,
		ExecutionPins: 2,
	})
	require.NoError(t, err)
	status, err := decodeNativeStatus(data)
	require.NoError(t, err)
	require.Equal(t, 1, status.ProcessClaims)
	require.Equal(t, 2, status.UncertainPins)
}

func TestNativeValidationRequiresAcknowledgementAndExactIdentity(t *testing.T) {
	target := browserruntime.Manifest{
		Version:  "fixture",
		Contract: 2,
		Archive:  browserruntime.Archive{SHA256: strings.Repeat("a", 64)},
	}
	identity := target.Identity()
	validated := nativeValidation{Validated: true, Runtime: &identity}
	encode := func(value nativeValidation) []byte {
		data, err := json.Marshal(value)
		require.NoError(t, err)
		return data
	}
	require.NoError(t, verifyNativeValidation(encode(validated), target))
	for _, change := range []func(*nativeValidation){
		func(v *nativeValidation) { v.Validated = false },
		func(v *nativeValidation) { v.Runtime = nil },
		func(v *nativeValidation) {
			v.Runtime = &synthetic.RuntimeIdentity{
				Version: "different", Contract: target.Contract, SHA256: target.Archive.SHA256,
			}
		},
	} {
		value := validated
		change(&value)
		require.Error(t, verifyNativeValidation(encode(value), target))
	}
	for _, value := range []string{"{}", "null", `{"validated":true}`, `{"validated":true,"runtime":null}`,
		`{"validated":true,"runtime":{},"sandboxed_local_journey":true}`} {
		require.Error(t, verifyNativeValidation([]byte(value), target))
	}
}
