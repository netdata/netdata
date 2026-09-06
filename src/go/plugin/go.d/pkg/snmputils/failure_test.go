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
	tests := map[string]struct {
		err    error
		reason string
	}{
		"no failure": {reason: ""},
		"cancelled": {err: context.Canceled,
			reason: "cancelled"},
		"deadline": {err: context.DeadlineExceeded,
			reason: "deadline"},
		"connection refused": {
			err:    &net.OpError{Op: "dial", Net: "udp", Err: syscall.ECONNREFUSED},
			reason: "connection_refused",
		},
		"DNS not found": {
			err:    &net.DNSError{Err: "SECRET", Name: "SECRET", IsNotFound: true},
			reason: "dns_not_found",
		},
		"wrong digest": {err: gosnmp.ErrWrongDigest,
			reason: "wrong_digest"},
		"unknown username": {err: gosnmp.ErrUnknownUsername,
			reason: "unknown_user"},
		"unsupported security level": {err: gosnmp.ErrUnknownSecurityLevel,
			reason: "unsupported_security_level"},
		"formatted timeout is unknown": {err: errors.New("request timeout (after 3 retries): SECRET"),
			reason: "unknown"},
		"formatted authentication error is unknown": {err: errors.New("authentication failed SECRET"),
			reason: "unknown"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.err
			if err != nil {
				err = fmt.Errorf("outer SECRET: %w", err)
			}
			failure := ClassifyFailure(err)
			require.Equal(t, tc.reason, failure.Reason)
			require.True(t, failure.Valid())
			data, err := json.Marshal(failure)
			require.NoError(t, err)
			require.NotContains(t, string(data), "SECRET")
		})
	}
}

func TestFailureAnnotationsPreserveErrors(t *testing.T) {
	tests := map[string]struct {
		packet    *gosnmp.SnmpPacket
		operation string
		want      Failure
	}{
		"typed cause": {operation: "get",
			want: Failure{Operation: "get", Reason: "cancelled"}},
		"packet status": {
			packet:    &gosnmp.SnmpPacket{Error: gosnmp.AuthorizationError, ErrorIndex: 2},
			operation: "system_identity",
			want: Failure{
				Operation:    "system_identity",
				Reason:       "packet_error",
				PacketStatus: uint8(gosnmp.AuthorizationError),
				ErrorIndex:   2,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cause := fmt.Errorf("SECRET: %w", context.Canceled)
			var annotated error
			if tc.packet != nil {
				annotated = WithPacketFailure(cause, tc.operation, tc.packet)
			} else {
				annotated = WithFailure(cause, tc.operation, "")
			}
			require.Equal(t, cause.Error(), annotated.Error())
			require.ErrorIs(t, annotated, context.Canceled)
			require.Equal(t, tc.want, ClassifyFailure(annotated))
		})
	}
}

func TestFailureValidation(t *testing.T) {
	tests := map[string]struct {
		failure Failure
		valid   bool
	}{
		"operation without failure":          {failure: Failure{Operation: "get"}},
		"packet error with success status":   {failure: Failure{Reason: "packet_error"}},
		"packet status without packet error": {failure: Failure{Reason: "unknown", PacketStatus: 1}},
		"index without failure":              {failure: Failure{ErrorIndex: 1}},
		"standard packet status": {
			failure: Failure{Operation: "get", Reason: "packet_error", PacketStatus: 18, ErrorIndex: 2},
			valid:   true,
		},
		"nonstandard packet status": {
			failure: Failure{Operation: "get", Reason: "packet_error", PacketStatus: 255, ErrorIndex: 2},
			valid:   true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) { require.Equal(t, tc.valid, tc.failure.Valid()) })
	}
}
