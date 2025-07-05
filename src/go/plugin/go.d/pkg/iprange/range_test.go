// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"math/big"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestV4Range_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantString string
	}{
		{
			name:       "single IP",
			input:      "192.0.2.0",
			wantString: "192.0.2.0-192.0.2.0",
		},
		{
			name:       "IP range",
			input:      "192.0.2.0-192.0.2.10",
			wantString: "192.0.2.0-192.0.2.10",
		},
		{
			name:       "CIDR /24",
			input:      "192.0.2.0/24",
			wantString: "192.0.2.1-192.0.2.254",
		},
		{
			name:       "subnet mask",
			input:      "192.0.2.0/255.255.255.0",
			wantString: "192.0.2.1-192.0.2.254",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, tt.wantString, r.String())
		})
	}
}

func TestV4Range_Family(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"single IP", "192.0.2.0"},
		{"IP range", "192.0.2.0-192.0.2.10"},
		{"CIDR", "192.0.2.0/24"},
		{"subnet mask", "192.0.2.0/255.255.255.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, V4Family, r.Family())
		})
	}
}

func TestV4Range_Size(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantSize int64
	}{
		{"single IP", "192.0.2.0", 1},
		{"IP range", "192.0.2.0-192.0.2.10", 11},
		{"CIDR /24", "192.0.2.0/24", 254},
		{"CIDR /31", "192.0.2.0/31", 2},
		{"CIDR /32", "192.0.2.0/32", 1},
		{"subnet mask /24", "192.0.2.0/255.255.255.0", 254},
		{"subnet mask /31", "192.0.2.0/255.255.255.254", 2},
		{"subnet mask /32", "192.0.2.0/255.255.255.255", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, big.NewInt(tt.wantSize), r.Size())
		})
	}
}

func TestV4Range_Contains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rangeStr  string
		ip        string
		wantFound bool
	}{
		{
			name:      "IP inside range",
			rangeStr:  "192.0.2.0-192.0.2.10",
			ip:        "192.0.2.5",
			wantFound: true,
		},
		{
			name:      "IP outside range",
			rangeStr:  "192.0.2.0-192.0.2.10",
			ip:        "192.0.2.55",
			wantFound: false,
		},
		{
			name:      "IP equals start",
			rangeStr:  "192.0.2.0-192.0.2.10",
			ip:        "192.0.2.0",
			wantFound: true,
		},
		{
			name:      "IP equals end",
			rangeStr:  "192.0.2.0-192.0.2.10",
			ip:        "192.0.2.10",
			wantFound: true,
		},
		{
			name:      "IPv6 address in IPv4 range",
			rangeStr:  "192.0.2.0-192.0.2.10",
			ip:        "2001:db8::",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.rangeStr)
			require.NoError(t, err)
			require.NotNil(t, r)

			ip, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err)

			assert.Equal(t, tt.wantFound, r.Contains(ip))
		})
	}
}

func TestV4Range_Iterate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"single IP", "192.0.2.0"},
		{"small range", "192.0.2.0-192.0.2.10"},
		{"CIDR /30", "192.0.2.0/30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			// Count addresses yielded by iterator
			var count int64
			for addr := range r.Iterate() {
				// Verify the address is valid and in range
				assert.True(t, addr.IsValid())
				assert.True(t, r.Contains(addr))
				count++
			}

			assert.Equal(t, r.Size().Int64(), count)
		})
	}
}

func TestV6Range_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantString string
	}{
		{
			name:       "single IP",
			input:      "2001:db8::",
			wantString: "2001:db8::-2001:db8::",
		},
		{
			name:       "IP range",
			input:      "2001:db8::-2001:db8::10",
			wantString: "2001:db8::-2001:db8::10",
		},
		{
			name:       "CIDR /126",
			input:      "2001:db8::/126",
			wantString: "2001:db8::1-2001:db8::2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, tt.wantString, r.String())
		})
	}
}

func TestV6Range_Family(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"single IP", "2001:db8::"},
		{"IP range", "2001:db8::-2001:db8::10"},
		{"CIDR", "2001:db8::/126"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, V6Family, r.Family())
		})
	}
}

func TestV6Range_Size(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantSize int64
	}{
		{"single IP", "2001:db8::", 1},
		{"IP range", "2001:db8::-2001:db8::10", 17},
		{"CIDR /120", "2001:db8::/120", 254},
		{"CIDR /127", "2001:db8::/127", 2},
		{"CIDR /128", "2001:db8::/128", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			assert.Equal(t, big.NewInt(tt.wantSize), r.Size())
		})
	}
}

func TestV6Range_Contains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rangeStr  string
		ip        string
		wantFound bool
	}{
		{
			name:      "IP inside range",
			rangeStr:  "2001:db8::-2001:db8::10",
			ip:        "2001:db8::5",
			wantFound: true,
		},
		{
			name:      "IP outside range",
			rangeStr:  "2001:db8::-2001:db8::10",
			ip:        "2001:db8::ff",
			wantFound: false,
		},
		{
			name:      "IP equals start",
			rangeStr:  "2001:db8::-2001:db8::10",
			ip:        "2001:db8::",
			wantFound: true,
		},
		{
			name:      "IP equals end",
			rangeStr:  "2001:db8::-2001:db8::10",
			ip:        "2001:db8::10",
			wantFound: true,
		},
		{
			name:      "IPv4 address in IPv6 range",
			rangeStr:  "2001:db8::-2001:db8::10",
			ip:        "192.0.2.0",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.rangeStr)
			require.NoError(t, err)
			require.NotNil(t, r)

			ip, err := netip.ParseAddr(tt.ip)
			require.NoError(t, err)

			assert.Equal(t, tt.wantFound, r.Contains(ip))
		})
	}
}

func TestV6Range_Iterate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"single IP", "2001:db8::5"},
		{"small range", "2001:db8::-2001:db8::10"},
		{"CIDR /124", "2001:db8::/124"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)
			require.NoError(t, err)
			require.NotNil(t, r)

			// Count addresses yielded by iterator
			var count int64
			for addr := range r.Iterate() {
				// Verify the address is valid and in range
				assert.True(t, addr.IsValid())
				assert.True(t, r.Contains(addr))
				count++
			}

			assert.Equal(t, r.Size().Int64(), count)
		})
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		start      string
		end        string
		wantNil    bool
		wantFamily Family
	}{
		{
			name:       "valid IPv4 range",
			start:      "192.0.2.0",
			end:        "192.0.2.10",
			wantFamily: V4Family,
		},
		{
			name:       "valid IPv6 range",
			start:      "2001:db8::",
			end:        "2001:db8::10",
			wantFamily: V6Family,
		},
		{
			name:    "IPv4 start > end",
			start:   "192.0.2.10",
			end:     "192.0.2.0",
			wantNil: true,
		},
		{
			name:    "IPv6 start > end",
			start:   "2001:db8::10",
			end:     "2001:db8::",
			wantNil: true,
		},
		{
			name:    "mixed families",
			start:   "192.0.2.0",
			end:     "2001:db8::",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, err := netip.ParseAddr(tt.start)
			require.NoError(t, err)

			end, err := netip.ParseAddr(tt.end)
			require.NoError(t, err)

			r := New(start, end)

			if tt.wantNil {
				assert.Nil(t, r)
			} else {
				require.NotNil(t, r)
				assert.Equal(t, tt.wantFamily, r.Family())
				assert.Equal(t, start, r.Start())
				assert.Equal(t, end, r.End())
			}
		})
	}
}

func TestFamily_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		family Family
		want   string
	}{
		{V4Family, "IPv4"},
		{V6Family, "IPv6"},
		{Family(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.family.String())
		})
	}
}

// Benchmark tests
func BenchmarkV4Range_Contains(b *testing.B) {
	r, _ := ParseRange("192.0.2.0/24")
	ip := netip.MustParseAddr("192.0.2.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Contains(ip)
	}
}

func BenchmarkV6Range_Contains(b *testing.B) {
	r, _ := ParseRange("2001:db8::/64")
	ip := netip.MustParseAddr("2001:db8::1234")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Contains(ip)
	}
}

func BenchmarkV4Range_Iterate(b *testing.B) {
	r, _ := ParseRange("192.0.2.0/24")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		for range r.Iterate() {
			count++
		}
	}
}
