// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"fmt"
	"math/big"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_String(t *testing.T) {
	tests := map[string]struct {
		input      string
		wantString string
	}{
		"singe": {
			input:      "192.0.2.0-192.0.2.10",
			wantString: "192.0.2.0-192.0.2.10",
		},
		"multiple": {
			input:      "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
			wantString: "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rs, err := ParseRanges(test.input)
			require.NoError(t, err)
			p := Pool(rs)

			assert.Equal(t, test.wantString, p.String())
		})
	}
}

func TestPool_Size(t *testing.T) {
	tests := map[string]struct {
		input    string
		wantSize *big.Int
	}{
		"singe": {
			input:    "192.0.2.0-192.0.2.10",
			wantSize: big.NewInt(11),
		},
		"multiple": {
			input:    "192.0.2.0-192.0.2.10 2001:db8::-2001:db8::10",
			wantSize: big.NewInt(11 + 17),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rs, err := ParseRanges(test.input)
			require.NoError(t, err)
			p := Pool(rs)

			assert.Equal(t, test.wantSize, p.Size())
		})
	}
}

func TestPool_Contains(t *testing.T) {
	tests := map[string]struct {
		input    string
		ip       string
		wantFail bool
	}{
		"inside first": {
			input: "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:    "192.0.2.5",
		},
		"inside last": {
			input: "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:    "2001:db8::5",
		},
		"outside": {
			input:    "192.0.2.0-192.0.2.10 192.0.2.20-192.0.2.30 2001:db8::-2001:db8::10",
			ip:       "192.0.2.100",
			wantFail: true,
		},
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (range: %s, ip: %s)", name, test.input, test.ip)
		t.Run(name, func(t *testing.T) {
			rs, err := ParseRanges(test.input)
			require.NoError(t, err)
			ip := net.ParseIP(test.ip)
			require.NotNil(t, ip)
			p := Pool(rs)

			if test.wantFail {
				assert.False(t, p.Contains(ip))
			} else {
				assert.True(t, p.Contains(ip))
			}
		})
	}
}
