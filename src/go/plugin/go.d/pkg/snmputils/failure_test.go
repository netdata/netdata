// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/require"
)

func TestClassifyFailureUsesTypedEvidence(t *testing.T) {
	tests := []struct {
		err    error
		reason string
	}{
		{nil, ""}, {context.Canceled, "cancelled"}, {context.DeadlineExceeded, "deadline"},
		{&net.OpError{Op: "dial", Net: "udp", Err: syscall.ECONNREFUSED}, "connection_refused"},
		{&net.DNSError{Err: "SECRET", Name: "SECRET", IsNotFound: true}, "dns_not_found"},
		{gosnmp.ErrWrongDigest, "wrong_digest"}, {gosnmp.ErrUnknownUsername, "unknown_user"},
		{gosnmp.ErrUnknownSecurityLevel, "unsupported_security_level"},
		{errors.New("request timeout (after 3 retries): SECRET"), "unknown"},
		{errors.New("authentication failed SECRET"), "unknown"},
	}
	for _, tt := range tests {
		err := tt.err
		if err != nil {
			err = fmt.Errorf("outer SECRET: %w", err)
		}
		f := ClassifyFailure(err)
		require.Equal(t, tt.reason, f.Reason)
		require.True(t, f.Valid())
		data, e := json.Marshal(f)
		require.NoError(t, e)
		require.NotContains(t, string(data), "SECRET")
	}
}

func TestFailureAnnotationsPreserveErrors(t *testing.T) {
	cause := fmt.Errorf("SECRET: %w", context.Canceled)
	annotated := WithFailure(cause, "get", "")
	require.Equal(t, cause.Error(), annotated.Error())
	require.ErrorIs(t, annotated, context.Canceled)
	require.Equal(t, Failure{Operation: "get", Reason: "cancelled"}, ClassifyFailure(annotated))
	packet := &gosnmp.SnmpPacket{Error: gosnmp.AuthorizationError, ErrorIndex: 2}
	annotated = WithPacketFailure(cause, "system_identity", packet)
	require.Equal(t, cause.Error(), annotated.Error())
	require.ErrorIs(t, annotated, context.Canceled)
	require.Equal(
		t,
		Failure{Operation: "system_identity", Reason: "packet_error", PacketStatus: uint8(gosnmp.AuthorizationError), ErrorIndex: 2},
		ClassifyFailure(annotated),
	)
}

func TestFailureRejectsContradictoryEvidence(t *testing.T) {
	for _, failure := range []Failure{
		{Operation: "get"}, {Reason: "packet_error"}, {Reason: "packet_error", PacketStatus: 255},
		{Reason: "unknown", PacketStatus: 1}, {Reason: "", ErrorIndex: 1},
	} {
		require.False(t, failure.Valid(), "%+v", failure)
	}
	require.True(t, (Failure{Operation: "get", Reason: "packet_error", PacketStatus: 18, ErrorIndex: 2}).Valid())
}
