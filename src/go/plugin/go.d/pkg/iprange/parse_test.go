// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantRanges []Range
		wantErr    bool
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "whitespace only",
			input: "   \t\n   ",
		},
		{
			name:  "single range",
			input: "192.0.2.0-192.0.2.10",
			wantRanges: []Range{
				mustParseRange(t, "192.0.2.0", "192.0.2.10"),
			},
		},
		{
			name:  "multiple ranges with different formats",
			input: "2001:db8::0 192.0.2.0-192.0.2.10 2001:db8::0/126 192.0.2.0/255.255.255.0",
			wantRanges: []Range{
				mustParseRange(t, "2001:db8::0", "2001:db8::0"),
				mustParseRange(t, "192.0.2.0", "192.0.2.10"),
				mustParseRange(t, "2001:db8::1", "2001:db8::2"),
				mustParseRange(t, "192.0.2.1", "192.0.2.254"),
			},
		},
		{
			name:    "single invalid syntax",
			input:   "192.0.2.0-192.0.2.",
			wantErr: true,
		},
		{
			name:    "multiple with one invalid",
			input:   "2001:db8::0 192.0.2.0-192.0.2.10 2001:db8::0/999 192.0.2.0/255.255.255.0",
			wantErr: true,
		},
		{
			name:  "extra whitespace",
			input: "  192.0.2.0   192.0.2.1-192.0.2.2  ",
			wantRanges: []Range{
				mustParseRange(t, "192.0.2.0", "192.0.2.0"),
				mustParseRange(t, "192.0.2.1", "192.0.2.2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ranges, err := ParseRanges(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ranges)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRanges, ranges)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantRange Range
		wantErr   error
	}{
		// Empty input
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "whitespace only",
			input: "   ",
		},

		// IPv4 single IP
		{
			name:      "IPv4 single IP",
			input:     "192.0.2.0",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.0"),
		},
		{
			name:    "IPv4 invalid address",
			input:   "192.0.2.",
			wantErr: ErrInvalidSyntax,
		},

		// IPv4 ranges
		{
			name:      "IPv4 range",
			input:     "192.0.2.0-192.0.2.10",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.10"),
		},
		{
			name:      "IPv4 range start equals end",
			input:     "192.0.2.0-192.0.2.0",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.0"),
		},
		{
			name:    "IPv4 range start > end",
			input:   "192.0.2.10-192.0.2.0",
			wantErr: ErrInvalidRange,
		},
		{
			name:    "IPv4 range invalid start",
			input:   "192.0.2.-192.0.2.10",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv4 range invalid end",
			input:   "192.0.2.0-192.0.2.",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv4 range with IPv6 start",
			input:   "2001:db8::0-192.0.2.10",
			wantErr: ErrMixedAddressFamilies,
		},
		{
			name:    "IPv4 range with IPv6 end",
			input:   "192.0.2.0-2001:db8::0",
			wantErr: ErrMixedAddressFamilies,
		},
		{
			name:      "IPv4 range with spaces",
			input:     " 192.0.2.0 - 192.0.2.10 ",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.10"),
		},

		// IPv4 CIDR
		{
			name:      "IPv4 CIDR /0",
			input:     "192.0.2.0/0",
			wantRange: mustParseRange(t, "0.0.0.1", "255.255.255.254"),
		},
		{
			name:      "IPv4 CIDR /24",
			input:     "192.0.2.0/24",
			wantRange: mustParseRange(t, "192.0.2.1", "192.0.2.254"),
		},
		{
			name:      "IPv4 CIDR /30",
			input:     "192.0.2.0/30",
			wantRange: mustParseRange(t, "192.0.2.1", "192.0.2.2"),
		},
		{
			name:      "IPv4 CIDR /31",
			input:     "192.0.2.0/31",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.1"),
		},
		{
			name:      "IPv4 CIDR /32",
			input:     "192.0.2.0/32",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.0"),
		},
		{
			name:      "IPv4 CIDR non-network address",
			input:     "192.0.2.10/24",
			wantRange: mustParseRange(t, "192.0.2.1", "192.0.2.254"),
		},
		{
			name:    "IPv4 CIDR missing prefix",
			input:   "192.0.2.0/",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv4 CIDR invalid prefix",
			input:   "192.0.2.0/99",
			wantErr: ErrInvalidSyntax,
		},

		// IPv4 subnet mask
		{
			name:      "IPv4 mask /0",
			input:     "192.0.2.0/0.0.0.0",
			wantRange: mustParseRange(t, "0.0.0.1", "255.255.255.254"),
		},
		{
			name:      "IPv4 mask /24",
			input:     "192.0.2.0/255.255.255.0",
			wantRange: mustParseRange(t, "192.0.2.1", "192.0.2.254"),
		},
		{
			name:      "IPv4 mask /30",
			input:     "192.0.2.0/255.255.255.252",
			wantRange: mustParseRange(t, "192.0.2.1", "192.0.2.2"),
		},
		{
			name:      "IPv4 mask /31",
			input:     "192.0.2.0/255.255.255.254",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.1"),
		},
		{
			name:      "IPv4 mask /32",
			input:     "192.0.2.0/255.255.255.255",
			wantRange: mustParseRange(t, "192.0.2.0", "192.0.2.0"),
		},
		{
			name:    "IPv4 mask invalid",
			input:   "192.0.2.0/mask",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv4 mask non-contiguous",
			input:   "192.0.2.0/255.255.0.254",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv4 mask with IPv6 address",
			input:   "2001:db8::/255.255.255.0",
			wantErr: ErrInvalidSyntax,
		},

		// IPv6 single IP
		{
			name:      "IPv6 single IP",
			input:     "2001:db8::0",
			wantRange: mustParseRange(t, "2001:db8::0", "2001:db8::0"),
		},
		{
			name:    "IPv6 invalid address",
			input:   "2001:db8",
			wantErr: ErrInvalidSyntax,
		},

		// IPv6 ranges
		{
			name:      "IPv6 range",
			input:     "2001:db8::-2001:db8::10",
			wantRange: mustParseRange(t, "2001:db8::", "2001:db8::10"),
		},
		{
			name:      "IPv6 range start equals end",
			input:     "2001:db8::-2001:db8::",
			wantRange: mustParseRange(t, "2001:db8::", "2001:db8::"),
		},
		{
			name:    "IPv6 range start > end",
			input:   "2001:db8::10-2001:db8::",
			wantErr: ErrInvalidRange,
		},
		{
			name:    "IPv6 range invalid start",
			input:   "2001:db8-2001:db8::10",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv6 range invalid end",
			input:   "2001:db8::-2001:db8",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv6 range with IPv4 start",
			input:   "192.0.2.0-2001:db8::10",
			wantErr: ErrMixedAddressFamilies,
		},
		{
			name:    "IPv6 range with IPv4 end",
			input:   "2001:db8::-192.0.2.10",
			wantErr: ErrMixedAddressFamilies,
		},

		// IPv6 CIDR
		{
			name:      "IPv6 CIDR /0",
			input:     "2001:db8::/0",
			wantRange: mustParseRange(t, "::1", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe"),
		},
		{
			name:      "IPv6 CIDR /64",
			input:     "2001:db8::/64",
			wantRange: mustParseRange(t, "2001:db8::1", "2001:db8::ffff:ffff:ffff:fffe"),
		},
		{
			name:      "IPv6 CIDR /126",
			input:     "2001:db8::/126",
			wantRange: mustParseRange(t, "2001:db8::1", "2001:db8::2"),
		},
		{
			name:      "IPv6 CIDR /127",
			input:     "2001:db8::/127",
			wantRange: mustParseRange(t, "2001:db8::", "2001:db8::1"),
		},
		{
			name:      "IPv6 CIDR /128",
			input:     "2001:db8::/128",
			wantRange: mustParseRange(t, "2001:db8::", "2001:db8::"),
		},
		{
			name:      "IPv6 CIDR non-network address",
			input:     "2001:db8::10/64",
			wantRange: mustParseRange(t, "2001:db8::1", "2001:db8::ffff:ffff:ffff:fffe"),
		},
		{
			name:    "IPv6 CIDR missing prefix",
			input:   "2001:db8::/",
			wantErr: ErrInvalidSyntax,
		},
		{
			name:    "IPv6 CIDR invalid prefix",
			input:   "2001:db8::/999",
			wantErr: ErrInvalidSyntax,
		},

		// Case sensitivity
		{
			name:      "mixed case IPv6",
			input:     "2001:DB8::A-2001:DB8::F",
			wantRange: mustParseRange(t, "2001:db8::a", "2001:db8::f"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, err := ParseRange(tt.input)

			if tt.wantErr != nil {
				assert.Error(t, err)
				if !errors.Is(err, tt.wantErr) {
					assert.ErrorContains(t, err, tt.wantErr.Error())
				}
				assert.Nil(t, r)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRange, r)
			}
		})
	}
}

func TestMaskToPrefixLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mask   string
		want   int
		wantOk bool
	}{
		{"all zeros", "0.0.0.0", 0, true},
		{"all ones", "255.255.255.255", 32, true},
		{"/8", "255.0.0.0", 8, true},
		{"/16", "255.255.0.0", 16, true},
		{"/24", "255.255.255.0", 24, true},
		{"/25", "255.255.255.128", 25, true},
		{"/30", "255.255.255.252", 30, true},
		{"/31", "255.255.255.254", 31, true},
		{"non-contiguous", "255.255.0.254", 0, false},
		{"holes in mask", "255.0.255.0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mask := netip.MustParseAddr(tt.mask)
			got, ok := maskToPrefixLen(mask)

			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// Helper function to create a range for testing
func mustParseRange(t *testing.T, start, end string) Range {
	t.Helper()

	startAddr, err := netip.ParseAddr(start)
	require.NoError(t, err)

	endAddr, err := netip.ParseAddr(end)
	require.NoError(t, err)

	r := New(startAddr, endAddr)
	require.NotNil(t, r)

	return r
}

// Benchmark tests
func BenchmarkParseRange_IPv4(b *testing.B) {
	inputs := []string{
		"192.0.2.1",
		"192.0.2.0-192.0.2.255",
		"192.0.2.0/24",
		"192.0.2.0/255.255.255.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseRange(inputs[i%len(inputs)])
	}
}

func BenchmarkParseRange_IPv6(b *testing.B) {
	inputs := []string{
		"2001:db8::1",
		"2001:db8::-2001:db8::ffff",
		"2001:db8::/64",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseRange(inputs[i%len(inputs)])
	}
}
