// SPDX-License-Identifier: GPL-3.0-or-later

package config

type Receiver struct {
	Listen         string    `yaml:"listen"               json:"listen"`   // default-secure 127.0.0.1:19938
	TLSCert        string    `yaml:"tls_cert"             json:"tls_cert"` // both or neither
	TLSKey         string    `yaml:"tls_key"              json:"tls_key"`
	TrustedProxies []string  `yaml:"trusted_proxies"      json:"trusted_proxies"` // CIDRs allowed to set XFF/X-Real-IP
	MaxBodyBytes   int64     `yaml:"max_body_bytes"       json:"max_body_bytes"`
	RateLimit      RateLimit `yaml:"rate_limit"           json:"rate_limit"`
	GeoIPDB        string    `yaml:"geoip_db"             json:"geoip_db"` // "" → agent topology-ip-intel mmdb
	// PublicURL is the receiver public base used by sites without their own override.
	PublicURL string `yaml:"public_url,omitempty" json:"public_url"`
}

type RateLimit struct {
	PerIPPerMin   int `yaml:"per_ip_per_min"   json:"per_ip_per_min"`
	PerSitePerSec int `yaml:"per_site_per_sec" json:"per_site_per_sec"`
}
