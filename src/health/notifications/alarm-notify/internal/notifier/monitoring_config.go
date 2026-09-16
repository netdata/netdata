// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func (dst Destination) validateMonitoring() error {
	allowed := Destination{Type: dst.Type, APIURL: dst.APIURL}
	if dst.Type == "alerta" {
		allowed.APIKey, allowed.Environment = dst.APIKey, dst.Environment
		if strings.TrimSpace(dst.Environment) == "" || strings.Contains(dst.Environment, "${") {
			return errors.New("alerta environment must be a nonempty literal")
		}
	} else {
		allowed.APIToken, allowed.EntitySelector = dst.APIToken, dst.EntitySelector
		allowed.EventType, allowed.Source = dst.EventType, dst.Source
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
	}
	if !reflect.DeepEqual(dst, allowed) {
		return fmt.Errorf("%s destination contains fields for another provider", dst.Type)
	}
	for _, field := range dst.monitoringSecrets() {
		reference, err := secret.IsReference(*field.value)
		if err != nil {
			return fmt.Errorf("%s %s: %w", dst.Type, field.name, err)
		}
		if !reference {
			if err := validateMonitoringField(dst.Type, field.name, *field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

type monitoringSecret struct {
	name  string
	value *string
}

func (dst *Destination) monitoringSecrets() []monitoringSecret {
	fields := []monitoringSecret{{"api_url", &dst.APIURL}}
	if dst.Type == "alerta" {
		return append(fields, monitoringSecret{"api_key", &dst.APIKey})
	}
	return append(fields, monitoringSecret{"api_token", &dst.APIToken})
}

func validateMonitoringField(provider, name, value string) error {
	if name == "api_url" {
		if value == "" {
			return fmt.Errorf("%s api_url is required", provider)
		}
		hosts := []string{""}
		if provider == "alerta" {
			// Public demo endpoints listed in Alerta's documentation; custom servers may use HTTP.
			hosts = []string{"api.alerta.io", "api.alerta.dev", "alerta-api.fly.dev"}
		}
		for _, host := range hosts {
			if err := validateAPIBase(value, provider, host); err != nil {
				return err
			}
		}
		return nil
	}
	if provider == "alerta" && value == "" {
		return nil // Alerta permits deployments without authentication.
	}
	return validateToken(value, provider, name)
}

func (dst *Destination) resolveMonitoring(ctx context.Context) error {
	for _, field := range dst.monitoringSecrets() {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("%s %s: %w", dst.Type, field.name, err)
		}
		if err := validateMonitoringField(dst.Type, field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
