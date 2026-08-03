// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net/netip"
	"strings"
	"testing"
)

func TestValidateListen(t *testing.T) {
	tests := map[string]struct {
		endpoints []Endpoint
		wantErr   string
	}{
		"valid single endpoint": {endpoints: []Endpoint{{Protocol: "udp", Address: "0.0.0.0", Port: 162}}},
		"valid IPv6 endpoint":   {endpoints: []Endpoint{{Protocol: "udp", Address: "::1", Port: 162}}},
		"valid multiple endpoints": {endpoints: []Endpoint{
			{Protocol: "udp", Address: "0.0.0.0", Port: 162},
			{Protocol: "udp", Address: "::1", Port: 3162},
		}},
		"duplicate endpoint": {endpoints: []Endpoint{
			{Protocol: "udp", Address: "127.0.0.1", Port: 162},
			{Protocol: "UDP", Address: "127.0.0.1", Port: 162},
		}, wantErr: "duplicate endpoint"},
		"empty endpoints":      {wantErr: "at least one endpoint"},
		"unsupported protocol": {endpoints: []Endpoint{{Protocol: "tcp", Address: "0.0.0.0", Port: 162}}, wantErr: "unsupported protocol"},
		"missing address":      {endpoints: []Endpoint{{Protocol: "udp", Port: 162}}, wantErr: "address is required"},
		"invalid port zero":    {endpoints: []Endpoint{{Protocol: "udp", Address: "0.0.0.0"}}, wantErr: "port must be"},
		"invalid port high":    {endpoints: []Endpoint{{Protocol: "udp", Address: "0.0.0.0", Port: 65536}}, wantErr: "port must be"},
		"invalid address":      {endpoints: []Endpoint{{Protocol: "udp", Address: "not-an-address", Port: 162}}, wantErr: "invalid address/port"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateListen(ListenConfig{Endpoints: tc.endpoints})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateListen failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestPolicyValidation(t *testing.T) {
	t.Run("valid versions", func(t *testing.T) {
		if _, err := NormalizeVersions([]string{"v1", "v2c", "v3"}); err != nil {
			t.Fatalf("NormalizeVersions failed: %v", err)
		}
	})

	t.Run("valid USM users", func(t *testing.T) {
		err := ValidateUSMUsers([]USMUser{{
			Username: "testuser", EngineID: testEngineIDHex,
			AuthProto: "sha256", AuthKey: "authsecret", PrivProto: "aes", PrivKey: "privsecret",
		}}, false)
		if err != nil {
			t.Fatalf("ValidateUSMUsers failed: %v", err)
		}
	})

	for name, user := range map[string]USMUser{
		"short auth key":     {Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "short"},
		"short priv key":     {Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "authpass", PrivProto: "aes", PrivKey: "short"},
		"invalid auth proto": {Username: "testuser", EngineID: testEngineIDHex, AuthProto: "whirlpool"},
		"invalid priv proto": {Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "authsecret", PrivProto: "3des"},
		"priv without auth":  {Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "aes", PrivKey: "privsecret"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateUSMUsers([]USMUser{user}, false); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	t.Run("static user requires engine ID", func(t *testing.T) {
		if err := ValidateUSMUsers([]USMUser{{Username: "testuser"}}, false); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("dynamic user may omit engine ID", func(t *testing.T) {
		if err := ValidateUSMUsers([]USMUser{{Username: "testuser"}}, true); err != nil {
			t.Fatalf("ValidateUSMUsers failed: %v", err)
		}
	})

	t.Run("invalid engine ID", func(t *testing.T) {
		if err := ValidateEngineIDWhitelist([]string{"nothex"}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("duplicate engine ID", func(t *testing.T) {
		if err := ValidateEngineIDWhitelist([]string{testEngineIDHex, strings.ToUpper(testEngineIDHex)}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("invalid source CIDR", func(t *testing.T) {
		if _, err := NormalizeSourceAllowlist([]string{"not-a-cidr"}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("trusted relays", func(t *testing.T) {
		prefixes, err := NormalizeTrustedRelays([]string{"10.0.0.0/8", "2001:db8::/32"})
		if err != nil {
			t.Fatalf("NormalizeTrustedRelays failed: %v", err)
		}
		if len(prefixes) != 2 {
			t.Fatalf("prefixes = %d, want 2", len(prefixes))
		}
	})

	t.Run("invalid trusted relay", func(t *testing.T) {
		if _, err := NormalizeTrustedRelays([]string{"not-a-cidr"}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("invalid rate limit mode", func(t *testing.T) {
		if err := ValidateRateLimit(RateLimitConfig{Enabled: true, PerSourcePPS: 100, Mode: "invalid"}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("dynamic discovery rejects whitelist", func(t *testing.T) {
		if err := ValidateDynamicEngineID(true, 0, []string{testEngineIDHex}); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("dynamic discovery rejects negative max", func(t *testing.T) {
		if err := ValidateDynamicEngineID(true, -1, nil); err == nil {
			t.Fatal("expected validation error")
		}
	})
}

func TestNormalizeCIDRListCanonicalizesIPv4MappedPrefixes(t *testing.T) {
	tests := map[string]func([]string) ([]netip.Prefix, error){
		"source allowlist": NormalizeSourceAllowlist,
		"trusted relays":   NormalizeTrustedRelays,
	}

	for name, normalize := range tests {
		t.Run(name, func(t *testing.T) {
			prefixes, err := normalize([]string{"::ffff:192.0.2.123/120", "2001:db8::1234/32"})
			if err != nil {
				t.Fatalf("normalize CIDRs: %v", err)
			}
			if got := prefixes[0].String(); got != "192.0.2.0/24" {
				t.Fatalf("mapped prefix = %q, want 192.0.2.0/24", got)
			}
			if !prefixes[0].Contains(netip.MustParseAddr("192.0.2.42")) {
				t.Fatal("canonical mapped prefix does not contain equivalent IPv4 peer")
			}
			if got := prefixes[1].String(); got != "2001:db8::/32" {
				t.Fatalf("native IPv6 prefix = %q, want 2001:db8::/32", got)
			}
		})
	}
}
