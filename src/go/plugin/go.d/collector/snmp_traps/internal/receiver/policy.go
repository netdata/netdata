// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

const (
	DefaultReceiveBuffer    = 4 * 1024 * 1024
	MaxReceiveBuffer        = 256 * 1024 * 1024
	defaultDynamicEngineMax = 4096
	minSNMPv3PassphraseLen  = 8
	defaultRateLimitPPS     = 1000
	defaultRateLimitMode    = "drop"
	defaultRateLimitSources = 10000
)

var defaultSourceCIDRs = []string{"0.0.0.0/0", "::/0"}

var validAuthProtos = map[string]bool{
	"none": true, "md5": true, "sha": true,
	"sha224": true, "sha256": true, "sha384": true, "sha512": true,
}

var validPrivProtos = map[string]bool{
	"none": true, "des": true, "aes": true,
	"aes192": true, "aes256": true, "aes192c": true, "aes256c": true,
}

type Endpoint struct {
	Protocol string
	Address  string
	Port     int
}

func (e Endpoint) LogName() string {
	return strings.ToLower(e.Protocol) + "://" + net.JoinHostPort(e.Address, strconv.Itoa(e.Port))
}

type ListenConfig struct {
	Endpoints     []Endpoint
	ReceiveBuffer int
}

type USMUser struct {
	Username  string
	EngineID  string
	AuthProto string
	AuthKey   string
	PrivProto string
	PrivKey   string
}

type RateLimitConfig struct {
	Enabled      bool
	PerSourcePPS int
	Mode         string
}

type PolicyConfig struct {
	Listen             ListenConfig
	Versions           []string
	Communities        []string
	USMUsers           []USMUser
	EngineIDWhitelist  []string
	LocalEngineID      string
	DynamicEngineID    bool
	DynamicEngineIDMax int
	SourceAllowlist    []netip.Prefix
	TrustedRelays      []netip.Prefix
	RateLimit          RateLimitConfig
}

type Policy struct {
	listen             ListenConfig
	versions           map[model.SnmpVersion]struct{}
	communities        []string
	users              []USMUser
	engineIDWhitelist  []string
	localEngineID      string
	dynamicEngineID    bool
	dynamicEngineIDMax int
	sourceAllowlist    []netip.Prefix
	trustedRelays      []netip.Prefix
	rateLimit          RateLimitConfig
}

func NewPolicy(cfg PolicyConfig) Policy {
	dynamicMax := cfg.DynamicEngineIDMax
	if dynamicMax == 0 {
		dynamicMax = defaultDynamicEngineMax
	}
	versions := make(map[model.SnmpVersion]struct{}, len(cfg.Versions))
	for _, version := range cfg.Versions {
		versions[model.SnmpVersion(version)] = struct{}{}
	}
	return Policy{
		listen:             cloneListenConfig(cfg.Listen),
		versions:           versions,
		communities:        slices.Clone(cfg.Communities),
		users:              slices.Clone(cfg.USMUsers),
		engineIDWhitelist:  slices.Clone(cfg.EngineIDWhitelist),
		localEngineID:      cfg.LocalEngineID,
		dynamicEngineID:    cfg.DynamicEngineID,
		dynamicEngineIDMax: dynamicMax,
		sourceAllowlist:    cloneCanonicalPrefixes(cfg.SourceAllowlist),
		trustedRelays:      cloneCanonicalPrefixes(cfg.TrustedRelays),
		rateLimit:          cfg.RateLimit,
	}
}

func (p Policy) V3Enabled() bool {
	_, ok := p.versions[model.SnmpVersionV3]
	return ok
}

func cloneListenConfig(cfg ListenConfig) ListenConfig {
	cfg.Endpoints = slices.Clone(cfg.Endpoints)
	return cfg
}

func cloneCanonicalPrefixes(prefixes []netip.Prefix) []netip.Prefix {
	if prefixes == nil {
		return nil
	}
	cloned := make([]netip.Prefix, len(prefixes))
	for i, prefix := range prefixes {
		cloned[i] = canonicalPrefix(prefix)
	}
	return cloned
}

func ValidateListen(cfg ListenConfig) error {
	if len(cfg.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}

	seen := make(map[string]struct{}, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		proto := strings.ToLower(ep.Protocol)
		if proto != "udp" {
			return fmt.Errorf("endpoint %d: unsupported protocol %q (only udp is supported)", i, proto)
		}
		if ep.Address == "" {
			return fmt.Errorf("endpoint %d: address is required", i)
		}
		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("endpoint %d: port must be between 1 and 65535, got %d", i, ep.Port)
		}

		addr := net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port))
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return fmt.Errorf("endpoint %d: invalid address/port %q: %v", i, addr, err)
		}
		key := proto + "/" + udpAddr.String()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("endpoint %d: duplicate endpoint %q", i, key)
		}
		seen[key] = struct{}{}
	}
	if cfg.ReceiveBuffer < 0 {
		return fmt.Errorf("listen.receive_buffer must be zero or positive, got %d", cfg.ReceiveBuffer)
	}
	if cfg.ReceiveBuffer > MaxReceiveBuffer {
		return fmt.Errorf("listen.receive_buffer must be <= %d, got %d", MaxReceiveBuffer, cfg.ReceiveBuffer)
	}
	return nil
}

func NormalizeVersions(versions []string) ([]string, error) {
	if len(versions) == 0 {
		return nil, errors.New("at least one SNMP version is required")
	}

	seen := make(map[string]struct{}, len(versions))
	normalized := make([]string, 0, len(versions))
	for i, version := range versions {
		version = strings.ToLower(strings.TrimSpace(version))
		switch version {
		case "v1", "v2c", "v3":
		default:
			return nil, fmt.Errorf("version %d: unsupported SNMP version %q (must be v1, v2c, or v3)", i, version)
		}
		if _, ok := seen[version]; ok {
			return nil, fmt.Errorf("version %d: duplicate SNMP version %q", i, version)
		}
		seen[version] = struct{}{}
		normalized = append(normalized, version)
	}
	return normalized, nil
}

type usmUserKey struct {
	username string
	engineID string
}

func ValidateUSMUsers(users []USMUser, dynamic bool) error {
	seen := make(map[usmUserKey]bool, len(users))
	for i, user := range users {
		if user.Username == "" {
			return fmt.Errorf("usm_users[%d]: username is required", i)
		}
		if user.EngineID == "" {
			if !dynamic {
				return fmt.Errorf("usm_users[%d]: engine_id is required for static v3 jobs", i)
			}
		} else if _, err := parseEngineIDHex(user.EngineID); err != nil {
			return fmt.Errorf("usm_users[%d]: engine_id: %w", i, err)
		}

		key := usmUserKey{username: user.Username, engineID: strings.ToLower(strings.TrimSpace(user.EngineID))}
		if seen[key] {
			return fmt.Errorf("usm_users[%d]: duplicate user %q for engine %q", i, user.Username, user.EngineID)
		}
		seen[key] = true

		authProto := strings.ToLower(user.AuthProto)
		if authProto == "" {
			authProto = "none"
		}
		if !validAuthProtos[authProto] {
			return fmt.Errorf("usm_users[%d]: invalid auth_proto %q (must be one of: none, md5, sha, sha224, sha256, sha384, sha512)", i, user.AuthProto)
		}
		privProto := strings.ToLower(user.PrivProto)
		if privProto == "" {
			privProto = "none"
		}
		if !validPrivProtos[privProto] {
			return fmt.Errorf("usm_users[%d]: invalid priv_proto %q (must be one of: none, des, aes, aes192, aes256, aes192c, aes256c)", i, user.PrivProto)
		}
		if authProto == "none" && privProto != "none" {
			return fmt.Errorf("usm_users[%d]: priv_proto %q requires auth_proto (noAuthNoPriv only supports none/none)", i, privProto)
		}
		if authProto != "none" {
			if user.AuthKey == "" {
				return fmt.Errorf("usm_users[%d]: auth_key is required when auth_proto is %q", i, authProto)
			}
			if len(user.AuthKey) < minSNMPv3PassphraseLen {
				return fmt.Errorf("usm_users[%d]: auth_key must be at least %d characters", i, minSNMPv3PassphraseLen)
			}
		}
		if privProto != "none" {
			if user.PrivKey == "" {
				return fmt.Errorf("usm_users[%d]: priv_key is required when priv_proto is %q", i, privProto)
			}
			if len(user.PrivKey) < minSNMPv3PassphraseLen {
				return fmt.Errorf("usm_users[%d]: priv_key must be at least %d characters", i, minSNMPv3PassphraseLen)
			}
		}
	}
	return nil
}

func ValidateEngineIDWhitelist(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if _, err := parseEngineIDHex(id); err != nil {
			return fmt.Errorf("engine_id_whitelist[%d]: %w", i, err)
		}
		key := strings.ToLower(strings.TrimSpace(id))
		if seen[key] {
			return fmt.Errorf("engine_id_whitelist[%d]: duplicate engine ID %q", i, id)
		}
		seen[key] = true
	}
	return nil
}

func ValidateLocalEngineID(id string) error {
	if id == "" {
		return nil
	}
	_, err := parseEngineIDHex(id)
	return err
}

func NormalizeSourceAllowlist(cidrs []string) ([]netip.Prefix, error) {
	if len(cidrs) == 0 {
		cidrs = defaultSourceCIDRs
	}
	return parseCIDRList("allowlist.source_cidrs", cidrs)
}

func NormalizeTrustedRelays(cidrs []string) ([]netip.Prefix, error) {
	return parseCIDRList("source.trusted_relays", cidrs)
}

func parseCIDRList(field string, cidrs []string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for i, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: invalid CIDR %q: %v", field, i, cidr, err)
		}
		prefixes = append(prefixes, canonicalPrefix(prefix))
	}
	return prefixes, nil
}

func canonicalPrefix(prefix netip.Prefix) netip.Prefix {
	if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
		return netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96).Masked()
	}
	return prefix.Masked()
}

func ValidateRateLimit(cfg RateLimitConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.PerSourcePPS < 0 {
		return fmt.Errorf("rate_limit.per_source_pps must be non-negative, got %d", cfg.PerSourcePPS)
	}
	mode := normalizeRateLimitMode(cfg.Mode)
	if mode != "drop" && mode != "sample" {
		return fmt.Errorf("rate_limit.mode must be 'drop' or 'sample', got %q", cfg.Mode)
	}
	return nil
}

func ValidateDynamicEngineID(enabled bool, max int, whitelist []string) error {
	if enabled && len(whitelist) > 0 {
		return errors.New("dynamic_engine_id_discovery and engine_id_whitelist are mutually exclusive; when dynamic discovery is enabled, engine_id_whitelist must be empty")
	}
	if max < 0 {
		return fmt.Errorf("dynamic_engine_id_max_pairs must be non-negative, got %d", max)
	}
	return nil
}

func ValidateV3Requirements(versions []string, users []USMUser, dynamic bool, whitelist []string) error {
	if !slices.Contains(versions, "v3") {
		return nil
	}
	if len(users) == 0 {
		return errors.New("SNMPv3 requires at least one usm_users entry")
	}
	if !dynamic && len(whitelist) == 0 {
		return errors.New("SNMPv3 requires engine_id_whitelist when dynamic_engine_id_discovery is disabled")
	}
	return nil
}

func parseEngineIDHex(id string) ([]byte, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("empty engine ID")
	}
	b, err := hex.DecodeString(id)
	if err != nil {
		return nil, fmt.Errorf("invalid hex %q: %w", id, err)
	}
	if len(b) < 5 || len(b) > 32 {
		return nil, fmt.Errorf("engine ID must be 5-32 bytes (got %d bytes)", len(b))
	}
	if isAllByte(b, 0x00) || isAllByte(b, 0xff) {
		return nil, errors.New("engine ID must not be all zeros or all 0xff bytes")
	}
	return b, nil
}
