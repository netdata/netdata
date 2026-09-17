// SPDX-License-Identifier: GPL-3.0-or-later

package alerta

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

func (dst Config) validate() error {
	if strings.TrimSpace(dst.Environment) == "" || strings.Contains(dst.Environment, "${") {
		return errors.New("alerta environment must be a nonempty literal")
	}
	for _, field := range dst.secretFields() {
		reference, err := secret.IsReference(*field.value)
		if err != nil {
			return fmt.Errorf("alerta %s: %w", field.name, err)
		}
		if !reference {
			if err := validateField(field.name, *field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

type secretField struct {
	name  string
	value *string
}

func (dst *Config) secretFields() []secretField {
	fields := []secretField{{"api_url", &dst.APIURL}}
	return append(fields, secretField{"api_key", &dst.APIKey})
}

func validateField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("alerta api_url is required")
		}
		hosts := []string{"api.alerta.io", "api.alerta.dev", "alerta-api.fly.dev"}
		for _, host := range hosts {
			if err := configfield.APIBase(value, "alerta", host); err != nil {
				return err
			}
		}
		return nil
	}
	if value == "" {
		return nil // Alerta permits deployments without authentication.
	}
	return configfield.Token(value, "alerta", name)
}

func (dst *Config) resolve(ctx context.Context) error {
	for _, field := range dst.secretFields() {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("alerta %s: %w", field.name, err)
		}
		if err := validateField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
