// SPDX-License-Identifier: GPL-3.0-or-later

// Package field validates reusable configuration scalar contracts.
package field

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"gopkg.in/yaml.v3"
)

// YAML normally truncates floats assigned to integers, which could select the wrong topic.
type Integer int64

func (value *Integer) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag != "!!int" {
		return errors.New("expected an integer")
	}
	var integer int64
	if err := node.Decode(&integer); err != nil {
		return err
	}
	*value = Integer(integer)
	return nil
}

func Token(value, provider, field string) error {
	if value == "" || strings.Contains(value, "${") ||
		strings.IndexFunc(value, func(r rune) bool { return r < 33 || r > 126 }) != -1 {
		return fmt.Errorf("%s %s must be nonempty printable ASCII without whitespace", provider, field)
	}
	return nil
}

func URL(value string) error {
	if !httpclient.ValidURL(value, false) {
		return errors.New("destination.url must be an absolute HTTP(S) URL without user information or fragment")
	}
	return nil
}

func APIBase(endpoint, provider, officialHost string) error {
	if endpoint == "" {
		return nil // The provider uses its official HTTPS endpoint.
	}
	// Even an empty fragment would capture the method appended to this base.
	if !httpclient.ValidURL(endpoint, false) || strings.Contains(endpoint, "#") {
		return fmt.Errorf(
			"%s api_url must be an absolute HTTP(S) base URL without user information or fragment",
			provider,
		)
	}
	u, _ := url.Parse(endpoint)
	if u.RawQuery != "" || u.ForceQuery {
		return fmt.Errorf("%s api_url must not contain a query", provider)
	}
	if strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), officialHost) && u.Scheme != "https" {
		return fmt.Errorf("the official %s API requires HTTPS", provider)
	}
	return nil
}

func PhoneNumber(value, provider, field string) error {
	digits := strings.TrimPrefix(value, "+")
	if digits == "" || strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return fmt.Errorf("%s %s must be one phone number using digits and an optional leading +", provider, field)
	}
	return nil
}
