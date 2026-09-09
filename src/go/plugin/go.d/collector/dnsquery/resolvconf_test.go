// SPDX-License-Identifier: GPL-3.0-or-later

package dnsquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseResolvConfNameservers(t *testing.T) {
	tests := map[string]struct {
		input string
		want  []string
	}{
		"empty": {},
		"search only": {
			input: "search example.com",
		},
		"single nameserver": {
			input: "  nameserver 1.2.3.4   ",
			want:  []string{"1.2.3.4"},
		},
		"multiple nameservers": {
			input: "nameserver 1.2.3.4\nnameserver 2001:db8::1\n",
			want:  []string{"1.2.3.4", "2001:db8::1"},
		},
		"commented nameserver": {
			input: "nameserver 1.2.3.4\n#nameserver 4.3.2.1",
			want:  []string{"1.2.3.4"},
		},
		"trailing comment": {
			input: "nameserver 1.2.3.4 # not 4.3.2.1",
			want:  []string{"1.2.3.4"},
		},
		"comment without whitespace is part of address": {
			input: "nameserver 1.2.3.4#comment",
		},
		"invalid nameserver": {
			input: "nameserver invalid",
		},
		"mixed valid and invalid nameservers": {
			input: "nameserver 1.2.3.4\nnameserver invalid\nnameserver 2001:db8::1",
			want:  []string{"1.2.3.4", "2001:db8::1"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, parseResolvConfNameservers([]byte(test.input)))
		})
	}
}

func TestUseSystemdResolvConf(t *testing.T) {
	assert.False(t, useSystemdResolvConf(nil))
	assert.False(t, useSystemdResolvConf([]string{"1.2.3.4"}))
	assert.False(t, useSystemdResolvConf([]string{systemdStubNameserverIP, "1.2.3.4"}))
	assert.True(t, useSystemdResolvConf([]string{systemdStubNameserverIP}))
}
