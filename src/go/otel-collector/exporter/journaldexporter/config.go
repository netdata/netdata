package journaldexporter

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
)

type Config struct {
	// Verbosity defines the journald exporter verbosity.
	Verbosity configtelemetry.Level `mapstructure:"verbosity,omitempty"`
}

var _ component.Config = (*Config)(nil)

func (cfg *Config) Validate() error {
	return nil
}
