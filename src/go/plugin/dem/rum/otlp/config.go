// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

// Config uses plaintext transport only when all TLS fields are unset.
type Config struct {
	Enabled   string `yaml:"enabled"            json:"enabled"`  // auto|yes|no
	Endpoint  string `yaml:"endpoint"           json:"endpoint"` // gRPC only (agent constraint)
	AuthToken string `yaml:"auth_token"         json:"auth_token"`
	TLSCert   string `yaml:"tls_cert,omitempty" json:"tls_cert"` // must be paired with tls_key
	TLSKey    string `yaml:"tls_key,omitempty"  json:"tls_key"`
	TLSCA     string `yaml:"tls_ca,omitempty"   json:"tls_ca"` // optional custom CA for the gRPC server cert
}
