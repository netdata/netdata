// SPDX-License-Identifier: GPL-3.0-or-later

package dynatrace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
)

func (dst Config) validate() error {
	if strings.TrimSpace(dst.EntitySelector) == "" || strings.Contains(dst.EntitySelector, "${") || utf8.RuneCountInString(dst.EntitySelector) > 2000 {
		return errors.New("dynatrace entity_selector must be a nonempty literal of at most 2000 characters")
	}
	switch dst.EventType {
	case "", "AVAILABILITY_EVENT", "CUSTOM_ALERT", "CUSTOM_ANNOTATION", "CUSTOM_CONFIGURATION", "CUSTOM_DEPLOYMENT", "CUSTOM_INFO", "ERROR_EVENT", "MARKED_FOR_TERMINATION", "PERFORMANCE_EVENT", "RESOURCE_CONTENTION_EVENT", "WARNING":
	default:
		return errors.New("dynatrace event_type must be a documented Events API v2 type")
	}
	if strings.Contains(dst.Source, "${") || utf8.RuneCountInString(dst.Source) > 4096 || dst.Source != "" && strings.TrimSpace(dst.Source) == "" {
		return errors.New("dynatrace source must be a nonempty literal of at most 4096 characters when set")
	}
	for _, field := range dst.secretFields() {
		reference, err := secret.IsReference(*field.value)
		if err != nil {
			return fmt.Errorf("dynatrace %s: %w", field.name, err)
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
	return append(fields, secretField{"api_token", &dst.APIToken})
}

func validateField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("dynatrace api_url is required")
		}
		return configfield.APIBase(value, "dynatrace", "")
	}
	return configfield.Token(value, "dynatrace", name)
}

func (dst *Config) resolve(ctx context.Context) error {
	for _, field := range dst.secretFields() {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("dynatrace %s: %w", field.name, err)
		}
		if err := validateField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
