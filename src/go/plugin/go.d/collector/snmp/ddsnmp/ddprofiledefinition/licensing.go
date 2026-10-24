// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"slices"
	"strings"
)

type LicenseSignalKind string

const (
	LicenseSignalExpiryTimestamp        LicenseSignalKind = "expiry_timestamp"
	LicenseSignalExpiryRemaining        LicenseSignalKind = "expiry_remaining"
	LicenseSignalAuthorizationTimestamp LicenseSignalKind = "authorization_timestamp"
	LicenseSignalAuthorizationRemaining LicenseSignalKind = "authorization_remaining"
	LicenseSignalCertificateTimestamp   LicenseSignalKind = "certificate_timestamp"
	LicenseSignalCertificateRemaining   LicenseSignalKind = "certificate_remaining"
	LicenseSignalGraceTimestamp         LicenseSignalKind = "grace_timestamp"
	LicenseSignalGraceRemaining         LicenseSignalKind = "grace_remaining"
	LicenseSignalUsageUsed              LicenseSignalKind = "usage_used"
	LicenseSignalUsageCapacity          LicenseSignalKind = "usage_capacity"
	LicenseSignalUsageAvailable         LicenseSignalKind = "usage_available"
	LicenseSignalUsagePercent           LicenseSignalKind = "usage_percent"
	LicenseSignalStateSeverity          LicenseSignalKind = "state_severity"
)

var validLicenseSignalKinds = map[LicenseSignalKind]struct{}{
	LicenseSignalExpiryTimestamp:        {},
	LicenseSignalExpiryRemaining:        {},
	LicenseSignalAuthorizationTimestamp: {},
	LicenseSignalAuthorizationRemaining: {},
	LicenseSignalCertificateTimestamp:   {},
	LicenseSignalCertificateRemaining:   {},
	LicenseSignalGraceTimestamp:         {},
	LicenseSignalGraceRemaining:         {},
	LicenseSignalUsageUsed:              {},
	LicenseSignalUsageCapacity:          {},
	LicenseSignalUsageAvailable:         {},
	LicenseSignalUsagePercent:           {},
	LicenseSignalStateSeverity:          {},
}

func IsValidLicenseSignalKind(kind LicenseSignalKind) bool {
	_, ok := validLicenseSignalKinds[kind]
	return ok
}

type LicenseSentinelPolicy string

const (
	LicenseSentinelTimerZeroOrNegative LicenseSentinelPolicy = "timer_zero_or_negative"
	LicenseSentinelTimerU32Max         LicenseSentinelPolicy = "timer_u32_max"
	LicenseSentinelTimerPre1971        LicenseSentinelPolicy = "timer_pre_1971"
)

var validLicenseSentinelPolicies = map[LicenseSentinelPolicy]struct{}{
	LicenseSentinelTimerZeroOrNegative: {},
	LicenseSentinelTimerU32Max:         {},
	LicenseSentinelTimerPre1971:        {},
}

func IsValidLicenseSentinelPolicy(policy LicenseSentinelPolicy) bool {
	_, ok := validLicenseSentinelPolicies[policy]
	return ok
}

type LicenseStatePolicy string

const (
	LicenseStatePolicyDefault LicenseStatePolicy = "default"
	LicenseStatePolicySophos  LicenseStatePolicy = "sophos"
)

var validLicenseStatePolicies = map[LicenseStatePolicy]struct{}{
	LicenseStatePolicyDefault: {},
	LicenseStatePolicySophos:  {},
}

func IsValidLicenseStatePolicy(policy LicenseStatePolicy) bool {
	_, ok := validLicenseStatePolicies[policy]
	return ok
}

type LicensingConfig struct {
	OriginProfileID string       `yaml:"-"               json:"-"`
	ID              string       `yaml:"id,omitempty"    json:"id,omitempty"`
	MIB             string       `yaml:"MIB,omitempty"   json:"MIB,omitempty"`
	Table           SymbolConfig `yaml:"table,omitempty" json:"table"`

	Identity    LicenseIdentityConfig    `yaml:"identity,omitempty"    json:"identity"`
	Descriptors LicenseDescriptorsConfig `yaml:"descriptors,omitempty" json:"descriptors"`
	State       LicenseStateConfig       `yaml:"state,omitempty"       json:"state"`
	Signals     LicenseSignalsConfig     `yaml:"signals,omitempty"     json:"signals"`

	StaticTags []StaticMetricTagConfig `yaml:"static_tags,omitempty" json:"-"`
	MetricTags []MetricTagConfig       `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
}

func (c LicensingConfig) Clone() LicensingConfig {
	return LicensingConfig{
		OriginProfileID: c.OriginProfileID,
		ID:              c.ID,
		MIB:             c.MIB,
		Table:           c.Table.Clone(),
		Identity:        c.Identity.Clone(),
		Descriptors:     c.Descriptors.Clone(),
		State:           c.State.Clone(),
		Signals:         c.Signals.Clone(),
		StaticTags:      slices.Clone(c.StaticTags),
		MetricTags:      cloneSlice(c.MetricTags),
	}
}

type LicenseIdentityConfig struct {
	ID        LicenseValueConfig `yaml:"id,omitempty"        json:"id"`
	Name      LicenseValueConfig `yaml:"name,omitempty"      json:"name"`
	Feature   LicenseValueConfig `yaml:"feature,omitempty"   json:"feature"`
	Component LicenseValueConfig `yaml:"component,omitempty" json:"component"`
}

func (c LicenseIdentityConfig) Clone() LicenseIdentityConfig {
	return LicenseIdentityConfig{
		ID:        c.ID.Clone(),
		Name:      c.Name.Clone(),
		Feature:   c.Feature.Clone(),
		Component: c.Component.Clone(),
	}
}

type LicenseDescriptorsConfig struct {
	Type      LicenseValueConfig `yaml:"type,omitempty"      json:"type"`
	Impact    LicenseValueConfig `yaml:"impact,omitempty"    json:"impact"`
	Perpetual LicenseValueConfig `yaml:"perpetual,omitempty" json:"perpetual"`
	Unlimited LicenseValueConfig `yaml:"unlimited,omitempty" json:"unlimited"`
}

func (c LicenseDescriptorsConfig) Clone() LicenseDescriptorsConfig {
	return LicenseDescriptorsConfig{
		Type:      c.Type.Clone(),
		Impact:    c.Impact.Clone(),
		Perpetual: c.Perpetual.Clone(),
		Unlimited: c.Unlimited.Clone(),
	}
}

type LicenseStateConfig struct {
	LicenseValueConfig `                   yaml:",inline"          json:",inline"`
	Policy             LicenseStatePolicy `yaml:"policy,omitempty" json:"policy,omitempty"`
}

func (c LicenseStateConfig) Clone() LicenseStateConfig {
	return LicenseStateConfig{
		LicenseValueConfig: c.LicenseValueConfig.Clone(),
		Policy:             c.Policy,
	}
}

type LicenseSignalsConfig struct {
	Expiry        LicenseTimerSignalsConfig `yaml:"expiry,omitempty"        json:"expiry"`
	Authorization LicenseTimerSignalsConfig `yaml:"authorization,omitempty" json:"authorization"`
	Certificate   LicenseTimerSignalsConfig `yaml:"certificate,omitempty"   json:"certificate"`
	Grace         LicenseTimerSignalsConfig `yaml:"grace,omitempty"         json:"grace"`
	Usage         LicenseUsageSignalsConfig `yaml:"usage,omitempty"         json:"usage"`
}

func (c LicenseSignalsConfig) Clone() LicenseSignalsConfig {
	return LicenseSignalsConfig{
		Expiry:        c.Expiry.Clone(),
		Authorization: c.Authorization.Clone(),
		Certificate:   c.Certificate.Clone(),
		Grace:         c.Grace.Clone(),
		Usage:         c.Usage.Clone(),
	}
}

type LicenseTimerSignalsConfig struct {
	LicenseValueConfig `                   yaml:",inline"             json:",inline"`
	Timestamp          LicenseValueConfig `yaml:"timestamp,omitempty" json:"timestamp"`
	Remaining          LicenseValueConfig `yaml:"remaining,omitempty" json:"remaining"`
}

func (c LicenseTimerSignalsConfig) Clone() LicenseTimerSignalsConfig {
	return LicenseTimerSignalsConfig{
		LicenseValueConfig: c.LicenseValueConfig.Clone(),
		Timestamp:          c.Timestamp.Clone(),
		Remaining:          c.Remaining.Clone(),
	}
}

type LicenseUsageSignalsConfig struct {
	Used      LicenseValueConfig `yaml:"used,omitempty"      json:"used"`
	Capacity  LicenseValueConfig `yaml:"capacity,omitempty"  json:"capacity"`
	Available LicenseValueConfig `yaml:"available,omitempty" json:"available"`
	Percent   LicenseValueConfig `yaml:"percent,omitempty"   json:"percent"`
}

func (c LicenseUsageSignalsConfig) Clone() LicenseUsageSignalsConfig {
	return LicenseUsageSignalsConfig{
		Used:      c.Used.Clone(),
		Capacity:  c.Capacity.Clone(),
		Available: c.Available.Clone(),
		Percent:   c.Percent.Clone(),
	}
}

type LicenseValueConfig struct {
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	From  string `yaml:"from,omitempty"  json:"from,omitempty"`

	Index          uint                   `yaml:"index,omitempty"           json:"index,omitempty"`
	IndexTransform []MetricIndexTransform `yaml:"index_transform,omitempty" json:"index_transform,omitempty"`

	Symbol SymbolConfig `yaml:"symbol,omitempty" json:"symbol"`
	OID    string       `yaml:"OID,omitempty"    json:"OID,omitempty"`
	Name   string       `yaml:"name,omitempty"   json:"name,omitempty"`

	Format   string                  `yaml:"format,omitempty"   json:"format,omitempty"`
	Mapping  MappingConfig           `yaml:"mapping,omitempty"  json:"mapping"`
	Sentinel []LicenseSentinelPolicy `yaml:"sentinel,omitempty" json:"sentinel,omitempty"`
	Kind     LicenseSignalKind       `yaml:"kind,omitempty"     json:"kind,omitempty"`
}

func (c LicenseValueConfig) IsSet() bool {
	return c.Value != "" ||
		c.From != "" ||
		c.Index != 0 ||
		len(c.IndexTransform) > 0 ||
		c.Symbol.OID != "" ||
		c.Symbol.Name != "" ||
		c.OID != "" ||
		c.Name != "" ||
		c.Format != "" ||
		c.Mapping.HasItems() ||
		c.Mapping.Mode != "" ||
		len(c.Sentinel) > 0 ||
		c.Kind != ""
}

func (c LicenseValueConfig) Clone() LicenseValueConfig {
	return LicenseValueConfig{
		Value:          c.Value,
		From:           c.From,
		Index:          c.Index,
		IndexTransform: slices.Clone(c.IndexTransform),
		Symbol:         c.Symbol.Clone(),
		OID:            c.OID,
		Name:           c.Name,
		Format:         c.Format,
		Mapping:        c.Mapping.Clone(),
		Sentinel:       slices.Clone(c.Sentinel),
		Kind:           c.Kind,
	}
}

// ForEachLicenseSignalValue visits configured signals in schema order.
func ForEachLicenseSignalValue(row LicensingConfig, add func(LicenseValueConfig)) {
	visitLicenseSignalValues(&row, func(_ string, value LicenseValueConfig) {
		if value.IsSet() {
			add(value)
		}
	}, nil)
}

func LicenseStructuralIdentity(row LicensingConfig) string {
	origin := row.OriginProfileID
	if origin == "" {
		origin = "<profile>"
	}
	return strings.Join([]string{origin, LicenseMergeIdentity(row)}, "|")
}

func LicenseMergeIdentity(row LicensingConfig) string {
	if row.Table.OID != "" {
		return strings.Join([]string{"table", TrimLicenseOID(row.Table.OID)}, "|")
	}
	if row.ID != "" {
		return strings.Join([]string{"scalar-group", row.ID}, "|")
	}
	var sourceOID string
	ForEachLicenseSignalValue(row, func(value LicenseValueConfig) {
		if sourceOID == "" {
			sourceOID = value.SourceOID()
		}
	})
	if sourceOID != "" {
		return strings.Join([]string{"scalar", TrimLicenseOID(sourceOID)}, "|")
	}
	return strings.Join([]string{"scalar", "<missing-source>"}, "|")
}

// SourceOID selects the OID used by collection, validation and inheritance identity.
func (value LicenseValueConfig) SourceOID() string {
	oid, _ := valueSourceOID(value.Symbol.OID, value.From, value.OID)
	return oid
}

// EffectiveSymbol applies symbol-first source and formatting defaults.
func (value LicenseValueConfig) EffectiveSymbol() SymbolConfig {
	sym := value.Symbol
	sym.OID = value.SourceOID()
	if sym.Name == "" {
		sym.Name = value.Name
	}
	if sym.Name == "" && sym.OID != "" {
		sym.Name = "license:" + strings.TrimPrefix(strings.TrimSpace(sym.OID), ".")
	}
	if sym.Format == "" {
		sym.Format = value.Format
	}
	if !sym.Mapping.HasItems() && value.Mapping.HasItems() {
		sym.Mapping = value.Mapping
	}
	return sym
}

func TrimLicenseOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

// visitLicenseSignalValues includes unset values so every consumer sees the same signal schema.
func visitLicenseSignalValues(
	row *LicensingConfig,
	read func(string, LicenseValueConfig),
	transform func(string, LicenseValueConfig) LicenseValueConfig,
) {
	add := func(path string, value *LicenseValueConfig) {
		if transform != nil {
			*value = transform(path, *value)
		} else {
			read(path, *value)
		}
	}
	add("state", &row.State.LicenseValueConfig)
	add("signals.expiry", &row.Signals.Expiry.LicenseValueConfig)
	add("signals.expiry.timestamp", &row.Signals.Expiry.Timestamp)
	add("signals.expiry.remaining", &row.Signals.Expiry.Remaining)
	add("signals.authorization", &row.Signals.Authorization.LicenseValueConfig)
	add("signals.authorization.timestamp", &row.Signals.Authorization.Timestamp)
	add("signals.authorization.remaining", &row.Signals.Authorization.Remaining)
	add("signals.certificate", &row.Signals.Certificate.LicenseValueConfig)
	add("signals.certificate.timestamp", &row.Signals.Certificate.Timestamp)
	add("signals.certificate.remaining", &row.Signals.Certificate.Remaining)
	add("signals.grace", &row.Signals.Grace.LicenseValueConfig)
	add("signals.grace.timestamp", &row.Signals.Grace.Timestamp)
	add("signals.grace.remaining", &row.Signals.Grace.Remaining)
	add("signals.usage.used", &row.Signals.Usage.Used)
	add("signals.usage.capacity", &row.Signals.Usage.Capacity)
	add("signals.usage.available", &row.Signals.Usage.Available)
	add("signals.usage.percent", &row.Signals.Usage.Percent)
}

func transformLicenseValues(row *LicensingConfig, visit func(string, LicenseValueConfig) LicenseValueConfig) {
	add := func(path string, value *LicenseValueConfig) { *value = visit(path, *value) }
	add("identity.id", &row.Identity.ID)
	add("identity.name", &row.Identity.Name)
	add("identity.feature", &row.Identity.Feature)
	add("identity.component", &row.Identity.Component)
	add("descriptors.type", &row.Descriptors.Type)
	add("descriptors.impact", &row.Descriptors.Impact)
	add("descriptors.perpetual", &row.Descriptors.Perpetual)
	add("descriptors.unlimited", &row.Descriptors.Unlimited)
	visitLicenseSignalValues(row, nil, visit)
}
