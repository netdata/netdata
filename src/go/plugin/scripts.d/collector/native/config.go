// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/configform"
	"gopkg.in/yaml.v2"
)

const (
	defaultUpdateEvery = 10
	defaultTimeout     = 5 * time.Second
	modeAuto           = "auto"
)

type jobModeSpec struct{}

func (jobModeSpec) Values() []string { return []string{modeAuto, modeOneshot, modePersistent} }
func (jobModeSpec) Default() string  { return modeAuto }

type jobSnapshotFormatSpec struct{}

func (jobSnapshotFormatSpec) Values() []string { return []string{modeAuto, formatJSON, formatLines} }
func (jobSnapshotFormatSpec) Default() string  { return modeAuto }

type Config struct {
	SnapshotFormat  confopt.Enum[jobSnapshotFormatSpec] `yaml:"snapshot_format,omitempty"     json:"snapshot_format,omitempty"`
	ScriptConfig    Settings                            `yaml:"config,omitempty"              json:"config,omitempty"`
	UpdateEvery     int                                 `yaml:"update_every,omitempty"        json:"update_every,omitempty"`
	AutoDetectEvery int                                 `yaml:"autodetection_retry,omitempty" json:"autodetection_retry,omitempty"`
	Command         []string                            `yaml:"command,omitempty"             json:"command,omitempty"`
	Mode            confopt.Enum[jobModeSpec]           `yaml:"mode,omitempty"                json:"mode,omitempty"`
	Manifest        string                              `yaml:"manifest,omitempty"            json:"manifest,omitempty"`
	Timeout         confopt.Duration                    `yaml:"timeout,omitempty"             json:"timeout,omitempty"`
}

// Settings holds a job's package configuration as JSON-compatible values,
// including nested objects decoded from YAML. Generic YAML jobs and registered
// package jobs share this decoder.
// Decode errors deliberately omit values, which may contain credentials.
type Settings map[string]any

var _ yaml.Unmarshaler = (*Settings)(nil)

func (s *Settings) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return errors.New("config must be a JSON-compatible object")
	}
	value, err := configform.Normalize(value)
	if err != nil {
		return err
	}
	if value == nil {
		*s = nil
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("config must be an object")
	}
	*s = object
	return nil
}

// effectiveSettings applies the package form defaults to a job's configuration.
func (d packageDefinition) effectiveSettings(settings Settings) Settings {
	if d.form == nil {
		return settings
	}
	return d.form.WithDefaults(settings)
}

// configEnvelope encodes a job's effective configuration as the first stdin
// line of every script invocation. Packages without a form receive no envelope.
func (d packageDefinition) configEnvelope(settings Settings) ([]byte, error) {
	if d.form == nil {
		if len(settings) != 0 {
			return nil, errors.New("package does not declare config_schema")
		}
		return nil, nil
	}
	effective := d.form.WithDefaults(settings)
	if err := d.form.Validate(effective); err != nil {
		return nil, err
	}
	data, err := json.Marshal(struct {
		Version string   `json:"version"`
		Config  Settings `json:"config"`
	}{
		Version: "v1",
		Config:  effective,
	})
	if err != nil {
		return nil, errors.New("config cannot be encoded")
	}
	if len(data)+1 > maxMessageBytes {
		return nil, errors.New("config envelope exceeds 64 MiB")
	}
	return append(data, '\n'), nil
}
