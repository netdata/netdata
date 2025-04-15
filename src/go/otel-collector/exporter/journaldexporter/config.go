// SPDX-License-Identifier: GPL-3.0-or-later

package journaldexporter

import (
	"time"

	"go.opentelemetry.io/collector/component"
)

type Config struct {
	URL     string        `mapstructure:"url"`
	Timeout time.Duration `mapstructure:"timeout"`
	TLS     struct {
		SrvCertFile        string `mapstructure:"server_certificate_file"`
		SrvKeyFile         string `mapstructure:"server_key_file"`
		TrustedCertFile    string `mapstructure:"trusted_certificate_file"`
		InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
	} `mapstructure:"tls"`
}

var _ component.Config = (*Config)(nil)

func (cfg *Config) Validate() error {
	return nil
}
