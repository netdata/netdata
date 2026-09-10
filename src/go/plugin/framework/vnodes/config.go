// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/snmpauth"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

// Config is authored input. Credentials must never enter VirtualNode snapshots.
type Config struct {
	VirtualNode `yaml:",inline"`
	Mode        string      `yaml:"mode,omitempty" json:"mode,omitempty"`
	ModeSNMP    *SNMPConfig `yaml:"mode_snmp,omitempty" json:"mode_snmp,omitempty"`
}
type SNMPConfig struct {
	snmpauth.Config `yaml:",inline"`
	Address         string           `yaml:"address" json:"address"`
	Port            int              `yaml:"port,omitempty" json:"port,omitempty"`
	Timeout         confopt.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Retries         *int             `yaml:"retries,omitempty" json:"retries,omitempty"`
}

// Metadata contains acquired values only; authored overrides are applied on publication.
type Metadata struct {
	Hostname string
	Labels   map[string]string
}

func (m *Metadata) Copy() *Metadata {
	if m == nil {
		return nil
	}
	return &Metadata{Hostname: m.Hostname, Labels: maps.Clone(m.Labels)}
}

// SNMPAcquirer may return usable metadata with an enrichment error. A nil result
// means no usable identity was acquired. Implementations must honor cancellation.
type SNMPAcquirer interface {
	Acquire(context.Context, SNMPConfig) (*Metadata, error)
}

func (c *Config) Copy() *Config {
	if c == nil {
		return nil
	}
	v := *c
	v.VirtualNode = *c.VirtualNode.Copy()
	if c.ModeSNMP != nil {
		s := c.ModeSNMP.Copy()
		v.ModeSNMP = &s
	}
	return &v
}
func (c SNMPConfig) Copy() SNMPConfig {
	c.Config = c.Config.Copy()
	if c.Retries != nil {
		v := *c.Retries
		c.Retries = &v
	}
	return c
}
func (c SNMPConfig) Defaults() SNMPConfig {
	c = c.Copy()
	c.Config = c.Config.Normalized()
	if c.Version == "" {
		c.Version = "2c"
	}
	if c.Port == 0 {
		c.Port = 161
	}
	if c.Timeout == 0 {
		c.Timeout = confopt.Duration(5 * time.Second)
	}
	if c.Retries == nil {
		v := 1
		c.Retries = &v
	}
	return c
}
func (c *Config) IsSNMP() bool { return c != nil && c.Mode == "snmp" }

// NormalizeCredentials removes inactive secrets before authored input is retained.
func (c *Config) NormalizeCredentials() {
	if c.IsSNMP() && c.ModeSNMP != nil {
		c.ModeSNMP.Config = c.ModeSNMP.Config.Normalized()
	}
}
func (c *Config) IdentityGUID() string {
	if c.GUID != "" || !c.IsSNMP() || c.ModeSNMP == nil {
		return c.GUID
	}
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(c.ModeSNMP.Address)).String()
}
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("configured vnode is nil")
	}
	switch c.Mode {
	case "", "static":
		if c.ModeSNMP != nil {
			return fmt.Errorf("mode_snmp requires mode snmp")
		}
		return ValidateConfigured(&c.VirtualNode)
	case "snmp":
		if c.ModeSNMP == nil {
			return fmt.Errorf("mode_snmp is required")
		}
	default:
		return fmt.Errorf("mode must be static or snmp")
	}
	s := c.ModeSNMP.Defaults()
	if strings.TrimSpace(s.Address) == "" {
		return fmt.Errorf("mode_snmp.address is required")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("mode_snmp.port must be between 1 and 65535")
	}
	if s.Timeout <= 0 {
		return fmt.Errorf("mode_snmp.timeout must be positive")
	}
	if *s.Retries < 0 {
		return fmt.Errorf("mode_snmp.retries cannot be negative")
	}
	if err := s.Config.Validate(); err != nil {
		return fmt.Errorf("mode_snmp: %w", err)
	}
	v := c.VirtualNode.Copy()
	v.GUID = c.IdentityGUID()
	if v.Hostname == "" {
		v.Hostname = c.Name
	}
	return ValidateConfigured(v)
}
func (c *Config) Resolve(m *Metadata) *VirtualNode {
	if c == nil || c.IsSNMP() && m == nil {
		return nil
	}
	v := c.VirtualNode.Copy()
	v.GUID = c.IdentityGUID()

	if c.IsSNMP() {
		if v.Hostname == "" {
			v.Hostname = strings.TrimSpace(m.Hostname)
		}
		if v.Hostname == "" {
			v.Hostname = c.Name
		}
		v.Labels = maps.Clone(m.Labels)
		if v.Labels == nil {
			v.Labels = make(map[string]string)
		}
		maps.Copy(v.Labels, c.Labels)
		// Normalize acquired fields with the effective hostname. Authored overrides
		// already passed strict validation, so preparation preserves their spelling.
		if prepared, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{GUID: v.GUID, Hostname: v.Hostname, Labels: v.Labels}); err == nil {
			v.Hostname = prepared.Hostname
			v.Labels = prepared.Labels
		}
	}

	return v
}

// ValidateUpdate preserves legacy static edits; SNMP identity replacement is explicit.
func (c *Config) ValidateUpdate(next *Config) error {
	if !c.IsSNMP() && !next.IsSNMP() {
		return nil
	}
	if c.IsSNMP() != next.IsSNMP() || c.ModeSNMP.Address != next.ModeSNMP.Address || c.ModeSNMP.ContextName() != next.ModeSNMP.ContextName() {
		return fmt.Errorf("changing vnode mode or SNMP target requires a replacement vnode")
	}
	a, _ := ConfiguredGUIDKey(c.IdentityGUID())
	b, _ := ConfiguredGUIDKey(next.IdentityGUID())
	if a != b {
		return fmt.Errorf("changing SNMP vnode GUID requires a replacement vnode")
	}
	return nil
}
func (c *Config) SameAcquisition(next *Config) bool {
	if !c.IsSNMP() || !next.IsSNMP() {
		return !c.IsSNMP() && !next.IsSNMP()
	}
	return reflect.DeepEqual(c.ModeSNMP.Defaults(), next.ModeSNMP.Defaults())
}

// ValidateConfigSet validates authored identities, including unresolved GUID reservations.
func ValidateConfigSet(initial map[string]*Config) error {
	guids := make(map[string]string)
	hostnames := make(map[string]string)
	for _, id := range slices.Sorted(maps.Keys(initial)) {
		c := initial[id]
		if c == nil {
			return fmt.Errorf("configured vnode %q is nil", id)
		}
		if c.Name != id {
			return fmt.Errorf("configured vnode %q identity differs from its map key", id)
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("configured vnode %q: %w", id, err)
		}
		guid, _ := ConfiguredGUIDKey(c.IdentityGUID())
		if other, ok := guids[guid]; ok {
			return fmt.Errorf("duplicate configured vnode GUID (%s and %s)", other, id)
		}
		guids[guid] = id
		if c.Hostname != "" {
			if other, ok := hostnames[c.Hostname]; ok {
				return fmt.Errorf("duplicate configured vnode hostname (%s and %s)", other, id)
			}
			hostnames[c.Hostname] = id
		}
	}
	return nil
}
