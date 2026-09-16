// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func (dst Destination) validateFormProvider() error {
	allowed := Destination{Type: dst.Type, APIKey: dst.APIKey, APIURL: dst.APIURL}
	if dst.Type == "kavenegar" {
		allowed.Sender, allowed.Recipient = dst.Sender, dst.Recipient
		for _, field := range []struct{ name, value string }{{"sender", dst.Sender}, {"recipient", dst.Recipient}} {
			if err := validatePhoneNumber(field.value, dst.Type, field.name); err != nil {
				return err
			}
		}
	}
	if !reflect.DeepEqual(dst, allowed) {
		return fmt.Errorf("%s destination contains fields for another provider", dst.Type)
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"api_key", dst.APIKey}} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("%s %s: %w", dst.Type, field.name, err)
		}
		if !reference {
			if err := validateFormProviderField(dst.Type, field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateFormProviderField(provider, name, value string) error {
	if name == "api_url" {
		host := "api.prowlapp.com"
		if provider == "kavenegar" {
			host = "api.kavenegar.com"
		}
		return validateAPIBase(value, provider, host)
	}
	if err := validateToken(value, provider, name); err != nil {
		return err
	}
	if provider == "prowl" {
		for key := range strings.SplitSeq(value, ",") {
			if _, err := hex.DecodeString(key); err != nil || len(key) != 40 {
				return fmt.Errorf("prowl api_key must contain comma-separated 40-character hexadecimal keys")
			}
		}
	} else if value == "." || value == ".." {
		return fmt.Errorf("kavenegar api_key must not be a dot path segment")
	}
	return nil
}

func (dst *Destination) resolveFormProvider(ctx context.Context) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"api_key", &dst.APIKey}} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("%s %s: %w", dst.Type, field.name, err)
		}
		if err := validateFormProviderField(dst.Type, field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}
