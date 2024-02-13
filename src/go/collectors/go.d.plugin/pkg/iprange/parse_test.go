// SPDX-License-Identifier: GPL-3.0-or-later

package iprange

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRanges(t *testing.T) {
	tests := map[string]struct {
		input      string
		wantRanges []Range
		wantErr    bool
	}{
		"single range": {
			input: "192.0.2.0-192.0.2.10",
			wantRanges: []Range{
				prepareRange("192.0.2.0", "192.0.2.10"),
			},
		},
		"multiple ranges": {
			input: "2001:db8::0 192.0.2.0-192.0.2.10 2001:db8::0/126 192.0.2.0/255.255.255.0",
			wantRanges: []Range{
				prepareRange("2001:db8::0", "2001:db8::0"),
				prepareRange("192.0.2.0", "192.0.2.10"),
				prepareRange("2001:db8::1", "2001:db8::2"),
				prepareRange("192.0.2.1", "192.0.2.254"),
			},
		},
		"single invalid syntax": {
			input:   "192.0.2.0-192.0.2.",
			wantErr: true,
		},
		"multiple invalid syntax": {
			input:   "2001:db8::0 192.0.2.0-192.0.2.10 2001:db8::0/999 192.0.2.0/255.255.255.0",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rs, err := ParseRanges(test.input)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nilf(t, rs, "want: nil, got: %s", rs)
			} else {
				assert.NoError(t, err)
				assert.Equalf(t, test.wantRanges, rs, "want: %s, got: %s", test.wantRanges, rs)
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantRange Range
		wantErr   bool
	}{
		"v4 IP": {
			input:     "192.0.2.0",
			wantRange: prepareRange("192.0.2.0", "192.0.2.0"),
		},
		"v4 IP: invalid address": {
			input:   "192.0.2.",
			wantErr: true,
		},
		"v4 Range": {
			input:     "192.0.2.0-192.0.2.10",
			wantRange: prepareRange("192.0.2.0", "192.0.2.10"),
		},
		"v4 Range: start == end": {
			input:     "192.0.2.0-192.0.2.0",
			wantRange: prepareRange("192.0.2.0", "192.0.2.0"),
		},
		"v4 Range: start > end": {
			input:   "192.0.2.10-192.0.2.0",
			wantErr: true,
		},
		"v4 Range: invalid start": {
			input:   "192.0.2.-192.0.2.10",
			wantErr: true,
		},
		"v4 Range: invalid end": {
			input:   "192.0.2.0-192.0.2.",
			wantErr: true,
		},
		"v4 Range: v6 start": {
			input:   "2001:db8::0-192.0.2.10",
			wantErr: true,
		},
		"v4 Range: v6 end": {
			input:   "192.0.2.0-2001:db8::0",
			wantErr: true,
		},
		"v4 CIDR: /0": {
			input:     "192.0.2.0/0",
			wantRange: prepareRange("0.0.0.1", "255.255.255.254"),
		},
		"v4 CIDR: /24": {
			input:     "192.0.2.0/24",
			wantRange: prepareRange("192.0.2.1", "192.0.2.254"),
		},
		"v4 CIDR: /30": {
			input:     "192.0.2.0/30",
			wantRange: prepareRange("192.0.2.1", "192.0.2.2"),
		},
		"v4 CIDR: /31": {
			input:     "192.0.2.0/31",
			wantRange: prepareRange("192.0.2.0", "192.0.2.1"),
		},
		"v4 CIDR: /32": {
			input:     "192.0.2.0/32",
			wantRange: prepareRange("192.0.2.0", "192.0.2.0"),
		},
		"v4 CIDR: ip instead of host address": {
			input:     "192.0.2.10/24",
			wantRange: prepareRange("192.0.2.1", "192.0.2.254"),
		},
		"v4 CIDR: missing prefix length": {
			input:   "192.0.2.0/",
			wantErr: true,
		},
		"v4 CIDR: invalid prefix length": {
			input:   "192.0.2.0/99",
			wantErr: true,
		},
		"v4 Mask: /0": {
			input:     "192.0.2.0/0.0.0.0",
			wantRange: prepareRange("0.0.0.1", "255.255.255.254"),
		},
		"v4 Mask: /24": {
			input:     "192.0.2.0/255.255.255.0",
			wantRange: prepareRange("192.0.2.1", "192.0.2.254"),
		},
		"v4 Mask: /30": {
			input:     "192.0.2.0/255.255.255.252",
			wantRange: prepareRange("192.0.2.1", "192.0.2.2"),
		},
		"v4 Mask: /31": {
			input:     "192.0.2.0/255.255.255.254",
			wantRange: prepareRange("192.0.2.0", "192.0.2.1"),
		},
		"v4 Mask: /32": {
			input:     "192.0.2.0/255.255.255.255",
			wantRange: prepareRange("192.0.2.0", "192.0.2.0"),
		},
		"v4 Mask: missing prefix mask": {
			input:   "192.0.2.0/",
			wantErr: true,
		},
		"v4 Mask: invalid mask": {
			input:   "192.0.2.0/mask",
			wantErr: true,
		},
		"v4 Mask: not canonical form mask": {
			input:   "192.0.2.0/255.255.0.254",
			wantErr: true,
		},
		"v4 Mask: v6 address": {
			input:   "2001:db8::/255.255.255.0",
			wantErr: true,
		},

		"v6 IP": {
			input:     "2001:db8::0",
			wantRange: prepareRange("2001:db8::0", "2001:db8::0"),
		},
		"v6 IP: invalid address": {
			input:   "2001:db8",
			wantErr: true,
		},
		"v6 Range": {
			input:     "2001:db8::-2001:db8::10",
			wantRange: prepareRange("2001:db8::", "2001:db8::10"),
		},
		"v6 Range: start == end": {
			input:     "2001:db8::-2001:db8::",
			wantRange: prepareRange("2001:db8::", "2001:db8::"),
		},
		"v6 Range: start > end": {
			input:   "2001:db8::10-2001:db8::",
			wantErr: true,
		},
		"v6 Range: invalid start": {
			input:   "2001:db8-2001:db8::10",
			wantErr: true,
		},
		"v6 Range: invalid end": {
			input:   "2001:db8::-2001:db8",
			wantErr: true,
		},
		"v6 Range: v4 start": {
			input:   "192.0.2.0-2001:db8::10",
			wantErr: true,
		},
		"v6 Range: v4 end": {
			input:   "2001:db8::-192.0.2.10",
			wantErr: true,
		},
		"v6 CIDR: /0": {
			input:     "2001:db8::/0",
			wantRange: prepareRange("::1", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe"),
		},
		"v6 CIDR: /64": {
			input:     "2001:db8::/64",
			wantRange: prepareRange("2001:db8::1", "2001:db8::ffff:ffff:ffff:fffe"),
		},
		"v6 CIDR: /126": {
			input:     "2001:db8::/126",
			wantRange: prepareRange("2001:db8::1", "2001:db8::2"),
		},
		"v6 CIDR: /127": {
			input:     "2001:db8::/127",
			wantRange: prepareRange("2001:db8::", "2001:db8::1"),
		},
		"v6 CIDR: /128": {
			input:     "2001:db8::/128",
			wantRange: prepareRange("2001:db8::", "2001:db8::"),
		},
		"v6 CIDR: ip instead of host address": {
			input:     "2001:db8::10/64",
			wantRange: prepareRange("2001:db8::1", "2001:db8::ffff:ffff:ffff:fffe"),
		},
		"v6 CIDR: missing prefix length": {
			input:   "2001:db8::/",
			wantErr: true,
		},
		"v6 CIDR: invalid prefix length": {
			input:   "2001:db8::/999",
			wantErr: true,
		},
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (%s)", name, test.input)
		t.Run(name, func(t *testing.T) {
			r, err := ParseRange(test.input)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nilf(t, r, "want: nil, got: %s", r)
			} else {
				assert.NoError(t, err)
				assert.Equalf(t, test.wantRange, r, "want: %s, got: %s", test.wantRange, r)
			}
		})
	}
}

func prepareRange(start, end string) Range {
	return New(net.ParseIP(start), net.ParseIP(end))
}
