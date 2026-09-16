// SPDX-License-Identifier: GPL-3.0-or-later

package redfishsd

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

const (
	defaultRescanInterval     = 30 * time.Minute
	defaultDeviceCacheTTL     = 12 * time.Hour
	defaultMaxConcurrentScans = 32
)

type Config struct {
	Source string `yaml:"-" json:"-"`

	RescanInterval    *confopt.LongDuration `yaml:"rescan_interval,omitempty" json:"rescan_interval,omitempty"`
	DeviceCacheTTL    *confopt.LongDuration `yaml:"device_cache_ttl,omitempty" json:"device_cache_ttl,omitempty"`
	MaxConcurrentScan int                   `yaml:"max_concurrent_scans,omitempty" json:"max_concurrent_scans,omitempty"`
	Profiles          []ProfileConfig       `yaml:"profiles" json:"profiles"`
	Networks          []NetworkConfig       `yaml:"networks" json:"networks"`
}

type ProfileConfig struct {
	Name         string         `yaml:"name" json:"name"`
	Scheme       string         `yaml:"scheme,omitempty" json:"scheme,omitempty"`
	JobConfig    map[string]any `yaml:"-" json:"-"`
	ProbeConfig  map[string]any `yaml:"-" json:"-"`
	hashRevision uint64
}

type NetworkConfig struct {
	Subnet  string `yaml:"subnet" json:"subnet"`
	Ports   []int  `yaml:"ports" json:"ports"`
	Profile string `yaml:"profile" json:"profile"`
}

type parsedConfig struct {
	rescanInterval     time.Duration
	scanOnce           bool
	deviceCacheTTL     time.Duration
	maxConcurrentScans int
	profiles           map[string]ProfileConfig
	networks           []network
}

type network struct {
	normalized  string
	addresses   []netip.Addr
	specificity int
	ports       []int
	profile     ProfileConfig
}

type normalizedNetwork struct {
	name        string
	addresses   []netip.Addr
	specificity int
	size        *big.Int
}

func (p *ProfileConfig) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if value, exists := raw["name"]; exists {
		text, ok := value.(string)
		if !ok {
			return errors.New("Redfish discovery profile 'name' must be a string")
		}
		p.Name = text
	}
	if value, exists := raw["scheme"]; exists {
		text, ok := value.(string)
		if !ok {
			return errors.New("Redfish discovery profile 'scheme' must be a string")
		}
		p.Scheme = text
	}
	delete(raw, "name")
	delete(raw, "scheme")
	p.JobConfig = raw
	return nil
}

func (c Config) validateAndParse() (parsedConfig, error) {
	result := parsedConfig{
		rescanInterval:     defaultRescanInterval,
		deviceCacheTTL:     defaultDeviceCacheTTL,
		maxConcurrentScans: defaultMaxConcurrentScans,
		profiles:           make(map[string]ProfileConfig),
	}
	if c.RescanInterval != nil {
		if c.RescanInterval.Duration() < 0 {
			return parsedConfig{}, errors.New("'rescan_interval' must be non-negative")
		}
		result.rescanInterval = c.RescanInterval.Duration()
		result.scanOnce = result.rescanInterval == 0
	}
	if c.DeviceCacheTTL != nil {
		if c.DeviceCacheTTL.Duration() < 0 {
			return parsedConfig{}, errors.New("'device_cache_ttl' must be non-negative")
		}
		result.deviceCacheTTL = c.DeviceCacheTTL.Duration()
	}
	if c.MaxConcurrentScan != 0 {
		result.maxConcurrentScans = c.MaxConcurrentScan
	}
	if result.maxConcurrentScans <= 0 {
		return parsedConfig{}, errors.New("'max_concurrent_scans' must be positive")
	}
	if len(c.Profiles) == 0 {
		return parsedConfig{}, errors.New("no Redfish discovery profiles provided")
	}
	secretResolver, err := secretresolver.NewDefaultAtomicResolver()
	if err != nil {
		return parsedConfig{}, fmt.Errorf("initialize discovery secret resolver: %w", err)
	}
	for index, profile := range c.Profiles {
		profile.Name = strings.TrimSpace(profile.Name)
		profile.Scheme = strings.ToLower(strings.TrimSpace(profile.Scheme))
		if profile.Name == "" {
			return parsedConfig{}, fmt.Errorf("profile %d has no name", index)
		}
		if _, ok := result.profiles[profile.Name]; ok {
			return parsedConfig{}, fmt.Errorf("duplicate profile name %q", profile.Name)
		}
		if profile.Scheme == "" {
			profile.Scheme = "https"
		}
		profile.JobConfig, err = redfish.PrepareDiscoveryProfile(profile.JobConfig, profile.Scheme)
		if err != nil {
			return parsedConfig{}, fmt.Errorf("profile %q: %w", profile.Name, err)
		}
		profile.ProbeConfig = cloneMap(profile.JobConfig)
		for _, key := range []string{"username", "password", "tls_cert", "tls_key"} {
			delete(profile.ProbeConfig, key)
		}
		profile.hashRevision, err = newProfileHashRevision()
		if err != nil {
			return parsedConfig{}, fmt.Errorf("profile %q target revision: %w", profile.Name, err)
		}
		if proxy, ok := profile.ProbeConfig["proxy_url"].(string); ok && strings.HasPrefix(strings.TrimSpace(proxy), "${") {
			resolved, err := secretResolver.Resolve(nil, proxy, nil)
			if err != nil {
				return parsedConfig{}, fmt.Errorf(
					"profile %q proxy_url secret reference could not be resolved",
					profile.Name,
				)
			}
			value, ok := resolved.(string)
			if !ok {
				return parsedConfig{}, fmt.Errorf("profile %q proxy_url did not resolve to a string", profile.Name)
			}
			profile.ProbeConfig["proxy_url"] = value
		}
		result.profiles[profile.Name] = profile
	}
	if len(c.Networks) == 0 {
		return parsedConfig{}, errors.New("no Redfish discovery networks provided")
	}

	byPrefix := make(map[string]int)
	for index, item := range c.Networks {
		item.Subnet = strings.TrimSpace(item.Subnet)
		item.Profile = strings.TrimSpace(item.Profile)
		profile, ok := result.profiles[item.Profile]
		if !ok {
			return parsedConfig{}, fmt.Errorf("network %d references unknown profile %q", index, item.Profile)
		}
		normalized, err := normalizeNetwork(item.Subnet)
		if err != nil {
			return parsedConfig{}, fmt.Errorf("network %d subnet %q: %w", index, item.Subnet, err)
		}
		if normalized.size.Cmp(big.NewInt(512)) > 0 {
			return parsedConfig{}, fmt.Errorf(
				"network %q exceeds the 512-address maximum (%s addresses)",
				normalized.name,
				normalized.size.String(),
			)
		}
		ports, err := normalizePorts(item.Ports)
		if err != nil {
			return parsedConfig{}, fmt.Errorf("network %q: %w", normalized.name, err)
		}
		key := normalized.name
		if prior, ok := byPrefix[key]; ok {
			existing := &result.networks[prior]
			if existing.profile.Name != profile.Name {
				return parsedConfig{}, fmt.Errorf(
					"network %q is assigned to both profile %q and %q",
					normalized.name, existing.profile.Name, profile.Name,
				)
			}
			existing.ports = unionPorts(existing.ports, ports)
			continue
		}
		byPrefix[key] = len(result.networks)
		result.networks = append(result.networks, network{
			normalized: normalized.name, addresses: normalized.addresses,
			specificity: normalized.specificity, ports: ports, profile: profile,
		})
	}
	sort.SliceStable(result.networks, func(i, j int) bool {
		if result.networks[i].specificity != result.networks[j].specificity {
			return result.networks[i].specificity > result.networks[j].specificity
		}
		return result.networks[i].normalized < result.networks[j].normalized
	})
	return result, nil
}

func newProfileHashRevision() (uint64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(raw[:]), nil
}

func normalizeNetwork(value string) (normalizedNetwork, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		if prefix.Addr().Is4In6() {
			if prefix.Bits() < 96 {
				return normalizedNetwork{}, errors.New("IPv4-mapped IPv6 prefix must be /96 or narrower")
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96).Masked()
		} else {
			prefix = prefix.Masked()
		}
		if prefix.Addr().Is6() && prefix.Addr().IsLinkLocalUnicast() {
			return normalizedNetwork{}, errors.New("link-local IPv6 discovery requires a zoned literal address")
		}
		return networkFromPrefix(prefix)
	}
	address, err := netip.ParseAddr(value)
	if err != nil {
		return normalizedNetwork{}, errors.New("must be a CIDR prefix or literal IP address")
	}
	zone := address.Zone()
	address = address.Unmap()
	if address.Is4() && zone != "" {
		return normalizedNetwork{}, errors.New("IPv4 address must not contain an interface zone")
	}
	if address.Is6() && address.IsLinkLocalUnicast() && address.Zone() == "" {
		return normalizedNetwork{}, errors.New("link-local IPv6 discovery requires a zoned literal address")
	}
	if address.Zone() != "" {
		return normalizedNetwork{
			name: address.String(), addresses: []netip.Addr{address}, specificity: 128, size: big.NewInt(1),
		}, nil
	}
	bits := 128
	if address.Is4() {
		bits = 32
	}
	return networkFromPrefix(netip.PrefixFrom(address, bits))
}

func networkFromPrefix(prefix netip.Prefix) (normalizedNetwork, error) {
	name := prefix.String()
	addresses, err := iprange.ParseRange(name)
	if err != nil {
		return normalizedNetwork{}, err
	}
	if addresses == nil {
		return normalizedNetwork{}, errors.New("produced no address range")
	}
	result := normalizedNetwork{
		name: name, specificity: prefix.Bits(), size: addresses.Size(),
	}
	if result.size.Cmp(big.NewInt(512)) <= 0 {
		for address := range addresses.Iterate() {
			result.addresses = append(result.addresses, address)
		}
	}
	return result, nil
}

func normalizePorts(values []int) ([]int, error) {
	if len(values) == 0 {
		return nil, errors.New("'ports' must contain at least one port")
	}
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, port := range values {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %s is outside 1..65535", strconv.Itoa(port))
		}
		if _, ok := seen[port]; ok {
			return nil, fmt.Errorf("duplicate port %d", port)
		}
		seen[port] = struct{}{}
		result = append(result, port)
	}
	sort.Ints(result)
	return result, nil
}

func unionPorts(left, right []int) []int {
	seen := make(map[int]struct{}, len(left)+len(right))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		seen[value] = struct{}{}
	}
	result := make([]int, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	maps.Copy(result, source)
	return result
}
