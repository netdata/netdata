// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyCatoError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"auth":       {err: errors.New("HTTP 403 forbidden"), want: "auth"},
		"rate limit": {err: errors.New("GraphQL rate limit exceeded"), want: "rate_limit"},
		"timeout":    {err: context.DeadlineExceeded, want: "timeout"},
		"canceled":   {err: fmt.Errorf("wrapped: %w", context.Canceled), want: "canceled"},
		"wrapped timeout": {
			err:  fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			want: "timeout",
		},
		"decode": {
			err:  errors.New("json: cannot unmarshal number into Go struct field"),
			want: "decode",
		},
		"network": {
			err:  &net.DNSError{Err: "no such host", Name: "api.invalid"},
			want: "network",
		},
		"tls": {
			err:  errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			want: "tls",
		},
		"proxy": {
			err:  errors.New("proxyconnect tcp: dial tcp: connection refused"),
			want: "proxy",
		},
		"empty": {
			err:  errors.New("no Cato sites discovered"),
			want: "empty",
		},
		"bare 403 is not enough for auth": {
			err:  errors.New("BGP ASN 403 has no session"),
			want: "error",
		},
		"bare 429 is not enough for rate limit": {
			err:  errors.New("429 routes reported by peer"),
			want: "error",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, classifyCatoError(tc.err))
		})
	}
}
