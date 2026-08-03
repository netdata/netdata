// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

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

type SourceConfig struct {
	TrustedRelays []string `yaml:"trusted_relays,omitempty" json:"trusted_relays"`
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

type JournalBackendConfig struct {
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled"`
}

func (c JournalBackendConfig) enabled() bool {
	return c.Enabled == nil || *c.Enabled
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

type ProfileMetricsConfig struct {
	Enabled bool     `yaml:"enabled,omitempty" json:"enabled"`
	Include []string `yaml:"include,omitempty" json:"include"`
}

type Config struct {
	Name               string               `yaml:"name,omitempty" json:"name"`
	Vnode              string               `yaml:"vnode,omitempty" json:"vnode"`
	ReverseDNS         ReverseDNSConfig     `yaml:"reverse_dns,omitempty" json:"reverse_dns"`
	UpdateEvery        int                  `yaml:"update_every,omitempty" json:"update_every"`
	Listen             ListenConfig         `yaml:"listen" json:"listen"`
	Versions           []string             `yaml:"versions,omitempty" json:"versions"`
	Communities        []string             `yaml:"communities,omitempty" json:"communities"`
	USMUsers           []USMUserConfig      `yaml:"usm_users,omitempty" json:"usm_users"`
	EngineIDWhitelist  []string             `yaml:"engine_id_whitelist,omitempty" json:"engine_id_whitelist"`
	LocalEngineID      string               `yaml:"local_engine_id,omitempty" json:"local_engine_id"`
	DynamicEngineID    bool                 `yaml:"dynamic_engine_id_discovery,omitempty" json:"dynamic_engine_id_discovery"`
	DynamicEngineIDMax int                  `yaml:"dynamic_engine_id_max_pairs,omitempty" json:"dynamic_engine_id_max_pairs"`
	Allowlist          AllowlistConfig      `yaml:"allowlist,omitempty" json:"allowlist"`
	Source             SourceConfig         `yaml:"source,omitempty" json:"source"`
	RateLimit          RateLimitConfig      `yaml:"rate_limit,omitempty" json:"rate_limit"`
	Dedup              DedupConfig          `yaml:"dedup,omitempty" json:"dedup"`
	Journal            JournalBackendConfig `yaml:"journal,omitempty" json:"journal"`
	OTLP               OTLPConfig           `yaml:"otlp,omitempty" json:"otlp"`
	Retention          jsonRetentionConfig  `yaml:"retention,omitempty" json:"retention"`
	Overrides          []OverrideConfig     `yaml:"overrides,omitempty" json:"overrides"`
	ProfileMetrics     ProfileMetricsConfig `yaml:"profile_metrics,omitempty" json:"profile_metrics"`
}

type ListenConfig struct {
	Endpoints     []EndpointConfig `yaml:"endpoints" json:"endpoints"`
	ReceiveBuffer int              `yaml:"receive_buffer,omitempty" json:"receive_buffer"`
}
