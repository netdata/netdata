// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"regexp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

const (
	maxJobNameLen = 64
)

var trapJobNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var labelKeyRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var (
	errJobNameEmpty   = errors.New("job name is empty")
	errJobNameTooLong = fmt.Errorf("job name exceeds %d characters", maxJobNameLen)
	errJobNameNoMatch = errors.New("job name must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$")
)

type validatedConfig struct {
	versions       []string
	trustedRelays  []netip.Prefix
	receiver       receiver.Policy
	otlp           otlp.Policy
	journalEnabled bool
	retention      journal.Retention
	dedup          dedup.Policy
	profileMetrics profilemetrics.Policy
}

func (c Config) Validate() error {
	_, err := validateConfig(c)
	return err
}

func validateConfig(c Config) (validatedConfig, error) {
	var validated validatedConfig

	if err := validateJobName(c.Name); err != nil {
		return validated, err
	}
	listen := toReceiverListenConfig(c.Listen)
	if err := receiver.ValidateListen(listen); err != nil {
		return validated, err
	}

	versions, err := receiver.NormalizeVersions(c.Versions)
	if err != nil {
		return validated, err
	}
	validated.versions = versions
	users := toReceiverUSMUsers(c.USMUsers)
	if err := receiver.ValidateUSMUsers(users, c.DynamicEngineID); err != nil {
		return validated, err
	}
	if err := receiver.ValidateEngineIDWhitelist(c.EngineIDWhitelist); err != nil {
		return validated, err
	}
	if err := receiver.ValidateLocalEngineID(c.LocalEngineID); err != nil {
		return validated, err
	}

	allowlist, err := receiver.NormalizeSourceAllowlist(c.Allowlist.SourceCIDRs)
	if err != nil {
		return validated, err
	}
	trustedRelays, err := receiver.NormalizeTrustedRelays(c.Source.TrustedRelays)
	if err != nil {
		return validated, err
	}
	validated.trustedRelays = trustedRelays

	if err := receiver.ValidateRateLimit(toReceiverRateLimitConfig(c.RateLimit)); err != nil {
		return validated, err
	}
	dedupPolicy, err := dedup.Normalize(dedup.Config{
		Enabled:         c.Dedup.Enabled,
		WindowSec:       c.Dedup.WindowSec,
		CacheMaxEntries: c.Dedup.CacheMaxEntries,
		KeyVarbinds:     c.Dedup.KeyVarbinds,
	})
	if err != nil {
		return validated, err
	}
	validated.dedup = dedupPolicy

	if c.OTLP.Enabled {
		policy, err := otlp.Normalize(otlp.Config{
			Endpoint:       c.OTLP.Endpoint,
			Headers:        maps.Clone(c.OTLP.Headers),
			RequestTimeout: c.OTLP.RequestTimeout,
			FlushInterval:  c.OTLP.FlushInterval,
			BatchSize:      c.OTLP.BatchSize,
			QueueCapacity:  c.OTLP.QueueCapacity,
		})
		if err != nil {
			return validated, err
		}
		validated.otlp = policy
	}
	validated.journalEnabled = c.Journal.enabled()
	if !validated.journalEnabled && !c.OTLP.Enabled {
		return validated, errors.New("at least one SNMP trap output backend must be enabled: journal.enabled or otlp.enabled")
	}

	if err := validateOverrides(c.Overrides); err != nil {
		return validated, err
	}
	if err := receiver.ValidateDynamicEngineID(c.DynamicEngineID, c.DynamicEngineIDMax, c.EngineIDWhitelist); err != nil {
		return validated, err
	}
	if err := receiver.ValidateV3Requirements(versions, users, c.DynamicEngineID, c.EngineIDWhitelist); err != nil {
		return validated, err
	}

	if validated.journalEnabled {
		retention, err := parseRetentionConfig(c.Retention)
		if err != nil {
			return validated, err
		}
		validated.retention = retention
	}

	profileMetrics, err := profilemetrics.Normalize(c.ProfileMetrics.Enabled, c.ProfileMetrics.Include)
	if err != nil {
		return validated, err
	}
	validated.profileMetrics = profileMetrics
	validated.receiver = receiver.NewPolicy(receiver.PolicyConfig{
		Listen:             listen,
		Versions:           versions,
		Communities:        c.Communities,
		USMUsers:           users,
		EngineIDWhitelist:  c.EngineIDWhitelist,
		LocalEngineID:      c.LocalEngineID,
		DynamicEngineID:    c.DynamicEngineID,
		DynamicEngineIDMax: c.DynamicEngineIDMax,
		SourceAllowlist:    allowlist,
		TrustedRelays:      trustedRelays,
		RateLimit:          toReceiverRateLimitConfig(c.RateLimit),
	})

	return validated, nil
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

func toReceiverListenConfig(cfg ListenConfig) receiver.ListenConfig {
	endpoints := make([]receiver.Endpoint, len(cfg.Endpoints))
	for i, endpoint := range cfg.Endpoints {
		endpoints[i] = receiver.Endpoint{
			Protocol: endpoint.Protocol,
			Address:  endpoint.Address,
			Port:     endpoint.Port,
		}
	}
	return receiver.ListenConfig{Endpoints: endpoints, ReceiveBuffer: cfg.ReceiveBuffer}
}

func toReceiverUSMUsers(users []USMUserConfig) []receiver.USMUser {
	out := make([]receiver.USMUser, len(users))
	for i, user := range users {
		out[i] = receiver.USMUser{
			Username:  user.Username,
			EngineID:  user.EngineID,
			AuthProto: user.AuthProto,
			AuthKey:   user.AuthKey,
			PrivProto: user.PrivProto,
			PrivKey:   user.PrivKey,
		}
	}
	return out
}

func toReceiverRateLimitConfig(cfg RateLimitConfig) receiver.RateLimitConfig {
	return receiver.RateLimitConfig{
		Enabled:      cfg.Enabled,
		PerSourcePPS: cfg.PerSourcePPS,
		Mode:         cfg.Mode,
	}
}

func validateOverrides(overrides []OverrideConfig) error {
	for i, o := range overrides {
		if o.OID == "" {
			return fmt.Errorf("overrides[%d]: oid is required", i)
		}
		if !model.IsNumericOID(o.OID) {
			return fmt.Errorf("overrides[%d]: invalid oid %q", i, o.OID)
		}
		if o.Category != "" && !catalog.ValidCategory(o.Category) {
			return fmt.Errorf("overrides[%d]: invalid category %q", i, o.Category)
		}
		if o.Severity != "" && !catalog.ValidSeverity(o.Severity) {
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
