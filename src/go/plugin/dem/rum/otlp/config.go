// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

type Config struct {
	Enabled   string `yaml:"enabled"            json:"enabled"`  // auto|yes|no
	Endpoint  string `yaml:"endpoint"           json:"endpoint"` // gRPC only (agent constraint)
	AuthToken string `yaml:"auth_token"         json:"auth_token"`
	TLSCert   string `yaml:"tls_cert,omitempty" json:"tls_cert"` // both or neither; plaintext when unset
	TLSKey    string `yaml:"tls_key,omitempty"  json:"tls_key"`
	TLSCA     string `yaml:"tls_ca,omitempty"   json:"tls_ca"` // optional custom CA for the gRPC server cert
}
