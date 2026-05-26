// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxJobNameLen          = 64
	minSNMPv3PassphraseLen = 8
)

var defaultAllowlistCIDRs = []string{"0.0.0.0/0", "::/0"}

var trapJobNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var labelKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var (
	errJobNameEmpty   = errors.New("job name is empty")
	errJobNameTooLong = fmt.Errorf("job name exceeds %d characters", maxJobNameLen)
	errJobNameNoMatch = errors.New("job name must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$")
)

var validAuthProtos = map[string]bool{
	"none": true, "md5": true, "sha": true,
	"sha224": true, "sha256": true, "sha384": true, "sha512": true,
}

var validPrivProtos = map[string]bool{
	"none": true, "des": true, "aes": true,
	"aes192": true, "aes256": true, "aes192c": true, "aes256c": true,
}

var validRateLimitModes = map[string]bool{
	"drop":   true,
	"sample": true,
}

func validateJobName(name string) error {
	if name == "" {
		return errJobNameEmpty
	}
	if len(name) > maxJobNameLen {
		return errJobNameTooLong
	}
	if !trapJobNameRE.MatchString(name) {
		return errJobNameNoMatch
	}
	return nil
}

func validateEndpoints(endpoints []EndpointConfig) error {
	if len(endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}

	seen := make(map[string]struct{}, len(endpoints))
	for i, ep := range endpoints {
		proto := strings.ToLower(ep.Protocol)
		switch proto {
		case "udp":
		default:
			return fmt.Errorf("endpoint %d: unsupported protocol %q (only udp is supported)", i, proto)
		}

		if ep.Address == "" {
			return fmt.Errorf("endpoint %d: address is required", i)
		}

		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("endpoint %d: port must be between 1 and 65535, got %d", i, ep.Port)
		}

		addrStr := net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port))
		udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			return fmt.Errorf("endpoint %d: invalid address/port %q: %v", i, addrStr, err)
		}
		key := proto + "/" + udpAddr.String()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("endpoint %d: duplicate endpoint %q", i, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateVersions(versions []string) ([]string, error) {
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

func validateUSMUsers(users []USMUserConfig) error {
	seen := make(map[usmUserKey]bool, len(users))
	for i, u := range users {
		if u.Username == "" {
			return fmt.Errorf("usm_users[%d]: username is required", i)
		}
		if _, err := parseEngineIDHex(u.EngineID); err != nil {
			return fmt.Errorf("usm_users[%d]: engine_id: %w", i, err)
		}
		key := usmUserKey{
			username: u.Username,
			engineID: strings.ToLower(strings.TrimSpace(u.EngineID)),
		}
		if seen[key] {
			return fmt.Errorf("usm_users[%d]: duplicate user %q for engine %q", i, u.Username, u.EngineID)
		}
		seen[key] = true

		authProto := strings.ToLower(u.AuthProto)
		if authProto == "" {
			authProto = "none"
		}
		if !validAuthProtos[authProto] {
			return fmt.Errorf("usm_users[%d]: invalid auth_proto %q (must be one of: none, md5, sha, sha224, sha256, sha384, sha512)", i, u.AuthProto)
		}

		privProto := strings.ToLower(u.PrivProto)
		if privProto == "" {
			privProto = "none"
		}
		if !validPrivProtos[privProto] {
			return fmt.Errorf("usm_users[%d]: invalid priv_proto %q (must be one of: none, des, aes, aes192, aes256, aes192c, aes256c)", i, u.PrivProto)
		}

		if authProto == "none" && privProto != "none" {
			return fmt.Errorf("usm_users[%d]: priv_proto %q requires auth_proto (noAuthNoPriv only supports none/none)", i, privProto)
		}

		if authProto != "none" {
			if u.AuthKey == "" {
				return fmt.Errorf("usm_users[%d]: auth_key is required when auth_proto is %q", i, authProto)
			}
			if len(u.AuthKey) < minSNMPv3PassphraseLen {
				return fmt.Errorf("usm_users[%d]: auth_key must be at least %d characters", i, minSNMPv3PassphraseLen)
			}
		}

		if privProto != "none" {
			if u.PrivKey == "" {
				return fmt.Errorf("usm_users[%d]: priv_key is required when priv_proto is %q", i, privProto)
			}
			if len(u.PrivKey) < minSNMPv3PassphraseLen {
				return fmt.Errorf("usm_users[%d]: priv_key must be at least %d characters", i, minSNMPv3PassphraseLen)
			}
		}
	}
	return nil
}

func validateEngineIDWhitelist(ids []string) error {
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

func validateLocalEngineID(hexStr string) error {
	if hexStr == "" {
		return nil
	}
	_, err := parseEngineIDHex(hexStr)
	return err
}

func validateAllowlist(al AllowlistConfig) ([]netip.Prefix, error) {
	if len(al.SourceCIDRs) == 0 {
		al.SourceCIDRs = defaultAllowlistCIDRs
	}
	var prefixes []netip.Prefix
	for i, cidr := range al.SourceCIDRs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, fmt.Errorf("allowlist.source_cidrs[%d]: invalid CIDR %q: %v", i, cidr, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func validateRateLimit(cfg RateLimitConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.PerSourcePPS < 0 {
		return fmt.Errorf("rate_limit.per_source_pps must be non-negative, got %d", cfg.PerSourcePPS)
	}
	mode := normalizeRateLimitMode(cfg.Mode)
	if !validRateLimitModes[mode] {
		return fmt.Errorf("rate_limit.mode must be 'drop' or 'sample', got %q", cfg.Mode)
	}
	return nil
}

func validateDeferredConfig(cfg Config) error {
	if cfg.DynamicEngineID {
		return errors.New("dynamic_engine_id_discovery is not implemented yet")
	}
	if len(cfg.Metrics) > 0 {
		return errors.New("metrics per-OID opt-in is not implemented yet")
	}
	return nil
}

func validateOverrides(overrides []OverrideConfig) error {
	for i, o := range overrides {
		if o.OID == "" {
			return fmt.Errorf("overrides[%d]: oid is required", i)
		}
		if !isNumericOID(o.OID) {
			return fmt.Errorf("overrides[%d]: invalid oid %q", i, o.OID)
		}
		if o.Category != "" && !validCategories[o.Category] {
			return fmt.Errorf("overrides[%d]: invalid category %q", i, o.Category)
		}
		if o.Severity != "" && !validSeverities[o.Severity] {
			return fmt.Errorf("overrides[%d]: invalid severity %q", i, o.Severity)
		}
		for key := range o.Labels {
			if err := validateConfigLabelKey(key); err != nil {
				return fmt.Errorf("overrides[%d]: label key %q: %w", i, key, err)
			}
		}
	}
	return nil
}

func validateConfigLabelKey(key string) error {
	if !labelKeyRE.MatchString(key) {
		return fmt.Errorf("does not match ^[a-z][a-z0-9_]*$")
	}
	return nil
}
