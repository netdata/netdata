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

func TestV4Range_String(t *testing.T) {
	tests := map[string]struct {
		input      string
		wantString string
	}{
		"IP":    {input: "192.0.2.0", wantString: "192.0.2.0-192.0.2.0"},
		"Range": {input: "192.0.2.0-192.0.2.10", wantString: "192.0.2.0-192.0.2.10"},
		"CIDR":  {input: "192.0.2.0/24", wantString: "192.0.2.1-192.0.2.254"},
		"Mask":  {input: "192.0.2.0/255.255.255.0", wantString: "192.0.2.1-192.0.2.254"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, test.wantString, r.String())
		})
	}
}

func TestV4Range_Family(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"IP":    {input: "192.0.2.0"},
		"Range": {input: "192.0.2.0-192.0.2.10"},
		"CIDR":  {input: "192.0.2.0/24"},
		"Mask":  {input: "192.0.2.0/255.255.255.0"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, V4Family, r.Family())
		})
	}
}

func TestV4Range_Size(t *testing.T) {
	tests := map[string]struct {
		input    string
		wantSize *big.Int
	}{
		"IP":      {input: "192.0.2.0", wantSize: big.NewInt(1)},
		"Range":   {input: "192.0.2.0-192.0.2.10", wantSize: big.NewInt(11)},
		"CIDR":    {input: "192.0.2.0/24", wantSize: big.NewInt(254)},
		"CIDR 31": {input: "192.0.2.0/31", wantSize: big.NewInt(2)},
		"CIDR 32": {input: "192.0.2.0/32", wantSize: big.NewInt(1)},
		"Mask":    {input: "192.0.2.0/255.255.255.0", wantSize: big.NewInt(254)},
		"Mask 31": {input: "192.0.2.0/255.255.255.254", wantSize: big.NewInt(2)},
		"Mask 32": {input: "192.0.2.0/255.255.255.255", wantSize: big.NewInt(1)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, test.wantSize, r.Size())
		})
	}
}

func TestV4Range_Contains(t *testing.T) {
	tests := map[string]struct {
		input    string
		ip       string
		wantFail bool
	}{
		"inside":   {input: "192.0.2.0-192.0.2.10", ip: "192.0.2.5"},
		"outside":  {input: "192.0.2.0-192.0.2.10", ip: "192.0.2.55", wantFail: true},
		"eq start": {input: "192.0.2.0-192.0.2.10", ip: "192.0.2.0"},
		"eq end":   {input: "192.0.2.0-192.0.2.10", ip: "192.0.2.10"},
		"v6":       {input: "192.0.2.0-192.0.2.10", ip: "2001:db8::", wantFail: true},
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (range: %s, ip: %s)", name, test.input, test.ip)
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)
			ip := net.ParseIP(test.ip)
			require.NotNil(t, ip)

			if test.wantFail {
				assert.False(t, r.Contains(ip))
			} else {
				assert.True(t, r.Contains(ip))
			}
		})
	}
}

func TestV4Range_Iterate(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"Single IP": {input: "192.0.2.0"},
		"IP range":  {input: "192.0.2.0-192.0.2.10"},
		"IP CIDR":   {input: "192.0.2.0/24"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			var n int64
			for range r.Iterate() {
				n++
			}
			assert.Equal(t, r.Size().Int64(), n)
		})
	}
}

func TestV6Range_String(t *testing.T) {
	tests := map[string]struct {
		input      string
		wantString string
	}{
		"IP":    {input: "2001:db8::", wantString: "2001:db8::-2001:db8::"},
		"Range": {input: "2001:db8::-2001:db8::10", wantString: "2001:db8::-2001:db8::10"},
		"CIDR":  {input: "2001:db8::/126", wantString: "2001:db8::1-2001:db8::2"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, test.wantString, r.String())
		})
	}
}

func TestV6Range_Family(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"IP":    {input: "2001:db8::"},
		"Range": {input: "2001:db8::-2001:db8::10"},
		"CIDR":  {input: "2001:db8::/126"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, V6Family, r.Family())
		})
	}
}

func TestV6Range_Size(t *testing.T) {
	tests := map[string]struct {
		input    string
		wantSize *big.Int
	}{
		"IP":       {input: "2001:db8::", wantSize: big.NewInt(1)},
		"Range":    {input: "2001:db8::-2001:db8::10", wantSize: big.NewInt(17)},
		"CIDR":     {input: "2001:db8::/120", wantSize: big.NewInt(254)},
		"CIDR 127": {input: "2001:db8::/127", wantSize: big.NewInt(2)},
		"CIDR 128": {input: "2001:db8::/128", wantSize: big.NewInt(1)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			assert.Equal(t, test.wantSize, r.Size())
		})
	}
}

func TestV6Range_Contains(t *testing.T) {
	tests := map[string]struct {
		input    string
		ip       string
		wantFail bool
	}{
		"inside":   {input: "2001:db8::-2001:db8::10", ip: "2001:db8::5"},
		"outside":  {input: "2001:db8::-2001:db8::10", ip: "2001:db8::ff", wantFail: true},
		"eq start": {input: "2001:db8::-2001:db8::10", ip: "2001:db8::"},
		"eq end":   {input: "2001:db8::-2001:db8::10", ip: "2001:db8::10"},
		"v4":       {input: "2001:db8::-2001:db8::10", ip: "192.0.2.0", wantFail: true},
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (range: %s, ip: %s)", name, test.input, test.ip)
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)
			ip := net.ParseIP(test.ip)
			require.NotNil(t, ip)

			if test.wantFail {
				assert.False(t, r.Contains(ip))
			} else {
				assert.True(t, r.Contains(ip))
			}
		})
	}
}

func TestV6Range_Iterate(t *testing.T) {
	tests := map[string]struct {
		input string
	}{
		"Single IP": {input: "2001:db8::5"},
		"IP range":  {input: "2001:db8::-2001:db8::10"},
		"IP CIDR":   {input: "2001:db8::/124"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)
			require.NoError(t, err)

			var n int64
			for range r.Iterate() {
				n++
			}
			assert.Equal(t, r.Size().Int64(), n)
		})
	}
}
