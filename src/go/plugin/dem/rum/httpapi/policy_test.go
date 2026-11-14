// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPTrustedProxies(t *testing.T) {
	snap := compile(&testCfg().Receiver)
	tests := map[string]struct {
		remote string
		hdr    map[string]string
		want   string
	}{
		"direct client, headers ignored": {
			remote: "203.0.113.5:4242", hdr: map[string]string{"X-Forwarded-For": "198.51.100.1"}, want: "203.0.113.5",
		},
		"trusted proxy, single xff": {
			remote: "127.0.0.1:4242", hdr: map[string]string{"X-Forwarded-For": "198.51.100.1"}, want: "198.51.100.1",
		},
		"trusted proxy, right-most non-trusted wins": {
			remote: "127.0.0.1:4242", hdr: map[string]string{"X-Forwarded-For": "1.1.1.1, 198.51.100.7, 10.1.2.3"}, want: "198.51.100.7",
		},
		"spoofed left entry does not win": {
			remote: "10.0.0.2:1", hdr: map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.8"}, want: "198.51.100.8",
		},
		"all hops trusted falls back to remote": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Forwarded-For": "10.9.9.9"}, want: "127.0.0.1",
		},
		"malformed xff falls back to remote": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Forwarded-For": "not-an-ip"}, want: "127.0.0.1",
		},
		"x-real-ip from trusted proxy": {
			remote: "127.0.0.1:1", hdr: map[string]string{"X-Real-IP": "198.51.100.9"}, want: "198.51.100.9",
		},
		"x-real-ip from untrusted ignored": {
			remote: "203.0.113.5:1", hdr: map[string]string{"X-Real-IP": "198.51.100.9"}, want: "203.0.113.5",
		},
		"ipv6 remote": {
			remote: "[2001:db8::1]:443", hdr: nil, want: "2001:db8::1",
		},
		"ipv6 loopback trusted": {
			remote: "[::1]:443", hdr: map[string]string{"X-Forwarded-For": "2001:db8::9"}, want: "2001:db8::9",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.hdr {
				r.Header.Set(k, v)
			}
			if got := snap.clientIP(r).String(); got != tc.want {
				t.Fatalf("clientIP = %s want %s", got, tc.want)
			}
		})
	}
}
