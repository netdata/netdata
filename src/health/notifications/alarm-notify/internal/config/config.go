// SPDX-License-Identifier: GPL-3.0-or-later

// Package config decodes configuration using explicitly supplied provider factories.
package config

import (
	"errors"
	"io"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"gopkg.in/yaml.v3"
)

type FactoryFunc func(*yaml.Node) (notifier.Sender, error)
type Registry map[string]FactoryFunc

// Factory binds strict typed decoding to a provider's validating constructor.
func Factory[C any](construct func(C) (notifier.Sender, error)) FactoryFunc {
	return func(node *yaml.Node) (notifier.Sender, error) {
		var destination struct {
			Type   string `yaml:"type"`
			Config C      `yaml:",inline"`
		}
		if err := decodeStrict(node, &destination); err != nil {
			return nil, invalidYAML()
		}
		return construct(destination.Config)
	}
}

func invalidYAML() error {
	// YAML errors may quote credentials in field names or values.
	return errors.New("invalid YAML configuration: check syntax, field names, and types")
}

func Read(r io.Reader, registry Registry) (notifier.Plan, error) {
	var root yaml.Node
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&root); err != nil {
		return notifier.Plan{}, invalidYAML()
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return notifier.Plan{}, errors.New("configuration must contain exactly one YAML document")
	}
	var document struct {
		Version      int                  `yaml:"version"`
		Destinations map[string]yaml.Node `yaml:"destinations"`
		Routing      notifier.Routing     `yaml:"routing,omitempty"`
	}
	if err := decodeStrict(&root, &document); err != nil {
		return notifier.Plan{}, invalidYAML()
	}
	if document.Version != 1 {
		return notifier.Plan{}, errors.New("configuration version must be 1")
	}
	if len(document.Destinations) == 0 {
		return notifier.Plan{}, errors.New("configuration requires at least one destination")
	}
	plan := notifier.Plan{Destinations: make(map[string]notifier.Sender, len(document.Destinations)), Routing: document.Routing}
	for name, node := range document.Destinations {
		if strings.TrimSpace(name) == "" {
			return notifier.Plan{}, errors.New("destination name must not be empty")
		}
		var discriminator struct {
			Type string `yaml:"type"`
		}
		if err := node.Decode(&discriminator); err != nil {
			return notifier.Plan{}, invalidYAML()
		}
		factory, ok := registry[discriminator.Type]
		if !ok {
			return notifier.Plan{}, errors.New("destination.type is not registered; other providers are not implemented yet")
		}
		sender, err := factory(&node)
		if err != nil {
			return notifier.Plan{}, err
		}
		plan.Destinations[name] = sender
	}
	if err := plan.Validate(); err != nil {
		return notifier.Plan{}, err
	}
	return plan, nil
}
