// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultAtomicResolver(t *testing.T) {
	tests := map[string]struct {
		input           func(t *testing.T) any
		want            any
		wantErrContains string
	}{
		"environment": {
			input: func(t *testing.T) any {
				t.Setenv("NETDATA_TEST_SECRET_ENV", "  secret \n")
				return "${env:NETDATA_TEST_SECRET_ENV}"
			},
			want: "secret",
		},
		"file": {
			input: func(t *testing.T) any {
				path := filepath.Join(t.TempDir(), "secret")
				if err := os.WriteFile(path, []byte("  value\n"), 0600); err != nil {
					t.Fatal(err)
				}
				return "${file:" + path + "}"
			},
			want: "value",
		},
		"multiple references": {
			input: func(t *testing.T) any {
				t.Setenv("NETDATA_TEST_SECRET_USER", "admin")
				t.Setenv("NETDATA_TEST_SECRET_PASSWORD", "password")
				return "${env:NETDATA_TEST_SECRET_USER}:${env:NETDATA_TEST_SECRET_PASSWORD}"
			},
			want: "admin:password",
		},
		"URI modifier": {
			input: func(t *testing.T) any {
				t.Setenv("NETDATA_TEST_SECRET_URI", "pa/ss+word: a@~")
				return "${env+urienc:NETDATA_TEST_SECRET_URI}"
			},
			want: "pa%2Fss%2Bword%3A%20a%40~",
		},
		"internal field is not resolved": {
			input: func(*testing.T) any {
				return map[string]any{
					"__source__": "${env:NETDATA_TEST_SECRET_MISSING}",
				}
			},
			want: map[string]any{
				"__source__": "${env:NETDATA_TEST_SECRET_MISSING}",
			},
		},
		"token without scheme is unchanged": {
			input: func(*testing.T) any { return "${NETDATA_TEST_SECRET}" },
			want:  "${NETDATA_TEST_SECRET}",
		},
		"missing environment variable": {
			input: func(*testing.T) any {
				return "${env:NETDATA_TEST_SECRET_MISSING}"
			},
			wantErrContains: "is not set",
		},
		"relative file": {
			input:           func(*testing.T) any { return "${file:relative/secret}" },
			wantErrContains: "file path must be absolute",
		},
		"unknown provider": {
			input:           func(*testing.T) any { return "${unknown:value}" },
			wantErrContains: "unknown provider",
		},
		"unknown modifier": {
			input:           func(*testing.T) any { return "${env+unknown:value}" },
			wantErrContains: "unknown output modifier",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resolver, err := NewDefaultAtomicResolver()
			if err != nil {
				t.Fatal(err)
			}
			got, err := resolver.Resolve(
				context.Background(),
				test.input(t),
				nil,
			)
			if test.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrContains) {
					t.Fatalf("error=%v want containing %q", err, test.wantErrContains)
				}
				if got != nil {
					t.Fatalf("failure returned result %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("result=%#v want %#v", got, test.want)
			}
		})
	}
}
