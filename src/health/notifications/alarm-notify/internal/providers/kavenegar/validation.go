// SPDX-License-Identifier: GPL-3.0-or-later

package kavenegar

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

func (dst Config) validate() error {
	for _, item := range []struct{ name, value string }{{"sender", dst.Sender}, {"recipient", dst.Recipient}} {
		if err := configfield.PhoneNumber(item.value, "kavenegar", item.name); err != nil {
			return err
		}
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"api_key", dst.APIKey}} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("kavenegar %s: %w", field.name, err)
		}
		if !reference {
			if err := validateField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateField(name, value string) error {
	if name == "api_url" {
		host := "api.kavenegar.com"
		return configfield.APIBase(value, "kavenegar", host)
	}
	if err := configfield.Token(value, "kavenegar", name); err != nil {
		return err
	}
	if value == "." || value == ".." {
		return fmt.Errorf("kavenegar api_key must not be a dot path segment")
	}
	return nil
}

func (dst *Config) resolve(ctx context.Context) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"api_key", &dst.APIKey}} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("kavenegar %s: %w", field.name, err)
		}
		if err := validateField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
