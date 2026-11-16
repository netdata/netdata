// SPDX-License-Identifier: GPL-3.0-or-later

package prowl

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

func (dst Config) validate() error {
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"api_key", dst.APIKey}} {
		reference, err := dst.Secrets.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("prowl %s: %w", field.name, err)
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
		host := "api.prowlapp.com"
		return configfield.APIBase(value, "prowl", host)
	}
	if err := configfield.Token(value, "prowl", name); err != nil {
		return err
	}
	for key := range strings.SplitSeq(value, ",") {
		if _, err := hex.DecodeString(key); err != nil || len(key) != 40 {
			return fmt.Errorf("prowl api_key must contain comma-separated 40-character hexadecimal keys")
		}
	}
	return nil
}

func (dst *Config) resolve(ctx context.Context) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"api_key", &dst.APIKey}} {
		value, err := dst.Secrets.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("prowl %s: %w", field.name, err)
		}
		if err := validateField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
