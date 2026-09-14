// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version      int                    `yaml:"version"`
	Destinations map[string]Destination `yaml:"destinations"`
	Routing      Routing                `yaml:"routing,omitempty"`
}

type Routing struct {
	Roles   map[string][]string `yaml:"roles,omitempty"`
	Default []string            `yaml:"default,omitempty"`
}

type Destination struct {
	Type        string `yaml:"type"`
	URL         string `yaml:"url"`
	BearerToken string `yaml:"bearer_token,omitempty"`
}

func readConfig(r io.Reader) (Config, error) {
	var cfg Config
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		// Decoder errors can quote credential-bearing scalar values and keys.
		return Config{}, errors.New("invalid YAML configuration: check syntax, field names, and types")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("configuration must contain exactly one YAML document")
	}
	if cfg.Version != 1 {
		return Config{}, errors.New("configuration version must be 1")
	}
	if len(cfg.Destinations) == 0 {
		return Config{}, errors.New("configuration requires at least one destination")
	}
	for name, dst := range cfg.Destinations {
		if strings.TrimSpace(name) == "" {
			return Config{}, errors.New("destination name must not be empty")
		}
		if err := dst.validate(); err != nil {
			return Config{}, err
		}
	}
	if err := cfg.validateRouting(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (dst Destination) validate() error {
	if dst.Type != "webhook" && dst.Type != "slack" {
		return errors.New("destination.type must be webhook or slack; other providers are not implemented yet")
	}
	if dst.Type == "slack" && dst.BearerToken != "" {
		return errors.New("slack destinations authenticate through their URL; bearer_token is not supported")
	}
	reference, err := secretReference(dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if !reference {
		if err := validateURL(dst.URL); err != nil {
			return err
		}
	}
	if _, err := secretReference(dst.BearerToken); err != nil {
		return fmt.Errorf("destination.bearer_token: %w", err)
	}
	if strings.ContainsAny(dst.BearerToken, "\r\n") {
		return errors.New("destination.bearer_token must not contain line breaks")
	}
	return nil
}

// Secret references occupy the whole value; configuration validation performs no resolution.
func secretReference(value string) (bool, error) {
	if !strings.Contains(value, "${") {
		return false, nil
	}
	scheme, operand, ok := strings.Cut(value, ":")
	if !ok || !strings.HasSuffix(operand, "}") {
		return false, errors.New("expected a whole env or file secret reference")
	}
	operand = strings.TrimSuffix(operand, "}")
	if strings.TrimSpace(operand) == "" || strings.ContainsAny(operand, "{}") {
		return false, errors.New("secret reference requires a nonempty operand without braces")
	}
	switch scheme {
	case "${env":
		return true, nil
	case "${file":
		if filepath.IsAbs(operand) {
			return true, nil
		}
		return false, errors.New("file secret reference requires an absolute path")
	default:
		return false, errors.New("only whole env and file secret references are supported")
	}
}

func validateURL(value string) error {
	if !validHTTPURL(value, false) {
		return errors.New("destination.url must be an absolute HTTP(S) URL without user information or fragment")
	}
	return nil
}

func validHTTPURL(value string, allowFragment bool) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && u.Opaque == "" && u.User == nil &&
		(allowFragment || u.Fragment == "") && (u.Scheme == "http" || u.Scheme == "https")
}
