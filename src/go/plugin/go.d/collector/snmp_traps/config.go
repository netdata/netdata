// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "fmt"

type EndpointConfig struct {
	Protocol string `yaml:"protocol" json:"protocol"`
	Address  string `yaml:"address" json:"address"`
	Port     int    `yaml:"port" json:"port"`
}

type USMUserConfig struct {
	Username  string `yaml:"username" json:"username"`
	EngineID  string `yaml:"engine_id" json:"engine_id"`
	AuthProto string `yaml:"auth_proto" json:"auth_proto"`
	AuthKey   string `yaml:"auth_key" json:"auth_key"`
	PrivProto string `yaml:"priv_proto" json:"priv_proto"`
	PrivKey   string `yaml:"priv_key" json:"priv_key"`
}

type AllowlistConfig struct {
	SourceCIDRs []string `yaml:"source_cidrs" json:"source_cidrs"`
}

type RateLimitConfig struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	PerSourcePPS int    `yaml:"per_source_pps" json:"per_source_pps"`
	Mode         string `yaml:"mode" json:"mode"`
}

type DedupConfig struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	WindowSec       int      `yaml:"window_sec" json:"window_sec"`
	CacheMaxEntries int      `yaml:"cache_max_entries" json:"cache_max_entries"`
	KeyVarbinds     []string `yaml:"key_varbinds,omitempty" json:"key_varbinds"`
}

type OTLPConfig struct {
	Enabled        bool              `yaml:"enabled" json:"enabled"`
	Endpoint       string            `yaml:"endpoint,omitempty" json:"endpoint"`
	Headers        map[string]string `yaml:"headers,omitempty" json:"headers"`
	RequestTimeout string            `yaml:"request_timeout,omitempty" json:"request_timeout"`
	FlushInterval  string            `yaml:"flush_interval,omitempty" json:"flush_interval"`
	BatchSize      int               `yaml:"batch_size,omitempty" json:"batch_size"`
	QueueCapacity  int               `yaml:"queue_capacity,omitempty" json:"queue_capacity"`
}

type OverrideConfig struct {
	OID      string            `yaml:"oid" json:"oid"`
	Category string            `yaml:"category,omitempty" json:"category"`
	Severity string            `yaml:"severity,omitempty" json:"severity"`
	Labels   map[string]string `yaml:"labels,omitempty" json:"labels"`
}

type ReverseDNSConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type MetricConfig struct {
	OID                  string `yaml:"oid" json:"oid"`
	Context              string `yaml:"context" json:"context"`
	DimensionFromVarbind string `yaml:"dimension_from_varbind,omitempty" json:"dimension_from_varbind"`
}

type Config struct {
	Vnode              string              `yaml:"vnode,omitempty" json:"vnode"`
	ReverseDNS         ReverseDNSConfig    `yaml:"reverse_dns,omitempty" json:"reverse_dns"`
	UpdateEvery        int                 `yaml:"update_every,omitempty" json:"update_every"`
	Listen             ListenConfig        `yaml:"listen" json:"listen"`
	Versions           []string            `yaml:"versions,omitempty" json:"versions"`
	Communities        []string            `yaml:"communities,omitempty" json:"communities"`
	USMUsers           []USMUserConfig     `yaml:"usm_users,omitempty" json:"usm_users"`
	EngineIDWhitelist  []string            `yaml:"engine_id_whitelist,omitempty" json:"engine_id_whitelist"`
	LocalEngineID      string              `yaml:"local_engine_id,omitempty" json:"local_engine_id"`
	DynamicEngineID    bool                `yaml:"dynamic_engine_id_discovery,omitempty" json:"dynamic_engine_id_discovery"`
	DynamicEngineIDMax int                 `yaml:"dynamic_engine_id_max_pairs,omitempty" json:"dynamic_engine_id_max_pairs"`
	Allowlist          AllowlistConfig     `yaml:"allowlist,omitempty" json:"allowlist"`
	RateLimit          RateLimitConfig     `yaml:"rate_limit,omitempty" json:"rate_limit"`
	Dedup              DedupConfig         `yaml:"dedup,omitempty" json:"dedup"`
	OTLP               OTLPConfig          `yaml:"otlp,omitempty" json:"otlp"`
	Retention          jsonRetentionConfig `yaml:"retention,omitempty" json:"retention"`
	Overrides          []OverrideConfig    `yaml:"overrides,omitempty" json:"overrides"`
	Metrics            []MetricConfig      `yaml:"metrics,omitempty" json:"metrics"`
}

type ListenConfig struct {
	Endpoints []EndpointConfig `yaml:"endpoints" json:"endpoints"`
}

type yamlKeySpec struct {
	children map[string]yamlKeySpec
	elem     *yamlKeySpec
	allowAny bool
}

var (
	endpointYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"protocol": {},
		"address":  {},
		"port":     {},
	}}

	usmUserYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"username":   {},
		"engine_id":  {},
		"auth_proto": {},
		"auth_key":   {},
		"priv_proto": {},
		"priv_key":   {},
	}}

	overrideYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"oid":      {},
		"category": {},
		"severity": {},
		"labels":   {allowAny: true},
	}}

	metricYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"oid":                    {},
		"context":                {},
		"dimension_from_varbind": {},
	}}

	configYAMLSpec = yamlKeySpec{children: map[string]yamlKeySpec{
		"vnode":                       {},
		"reverse_dns":                 {children: map[string]yamlKeySpec{"enabled": {}}},
		"update_every":                {},
		"listen":                      {children: map[string]yamlKeySpec{"endpoints": {elem: &endpointYAMLSpec}}},
		"versions":                    {},
		"communities":                 {},
		"usm_users":                   {elem: &usmUserYAMLSpec},
		"engine_id_whitelist":         {},
		"local_engine_id":             {},
		"dynamic_engine_id_discovery": {},
		"dynamic_engine_id_max_pairs": {},
		"allowlist":                   {children: map[string]yamlKeySpec{"source_cidrs": {}}},
		"rate_limit":                  {children: map[string]yamlKeySpec{"enabled": {}, "per_source_pps": {}, "mode": {}}},
		"dedup":                       {children: map[string]yamlKeySpec{"enabled": {}, "window_sec": {}, "cache_max_entries": {}, "key_varbinds": {}}},
		"otlp":                        {children: map[string]yamlKeySpec{"enabled": {}, "endpoint": {}, "headers": {allowAny: true}, "request_timeout": {}, "flush_interval": {}, "batch_size": {}, "queue_capacity": {}}},
		"retention":                   {children: map[string]yamlKeySpec{"max_size": {}, "max_duration": {}, "rotation_size": {}, "rotation_duration": {}}},
		"overrides":                   {elem: &overrideYAMLSpec},
		"metrics":                     {elem: &metricYAMLSpec},
	}}
)

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if err := rejectUnknownYAMLKeys(raw, configYAMLSpec, ""); err != nil {
		return err
	}

	type plain Config
	return unmarshal((*plain)(c))
}

func rejectUnknownYAMLKeys(node any, spec yamlKeySpec, path string) error {
	if spec.allowAny || node == nil {
		return nil
	}

	switch v := node.(type) {
	case map[interface{}]interface{}:
		if spec.children == nil {
			return nil
		}
		for rawKey, rawValue := range v {
			key, ok := rawKey.(string)
			if !ok {
				if path == "" {
					return fmt.Errorf("config key %v is not a string", rawKey)
				}
				return fmt.Errorf("%s: config key %v is not a string", path, rawKey)
			}
			child, ok := spec.children[key]
			if !ok {
				if path == "" {
					return fmt.Errorf("unknown config key %q", key)
				}
				return fmt.Errorf("%s: unknown config key %q", path, key)
			}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if err := rejectUnknownYAMLKeys(rawValue, child, childPath); err != nil {
				return err
			}
		}
	case []interface{}:
		if spec.elem == nil {
			return nil
		}
		for i, item := range v {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := rejectUnknownYAMLKeys(item, *spec.elem, itemPath); err != nil {
				return err
			}
		}
	}

	return nil
}
