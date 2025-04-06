package journaldexporter

import (
	"go.opentelemetry.io/collector/component"
)

type Config struct {
	URL                    string `mapstructure:"url"`
	ServerKeyFile          string `mapstructure:"server_key_file"`
	ServerCertificateFile  string `mapstructure:"server_certificate_file"`
	TrustedCertificateFile string `mapstructure:"trusted_certificate_file"`
}

var _ component.Config = (*Config)(nil)

func (cfg *Config) Validate() error {
	return nil
}
