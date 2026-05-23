// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"encoding/json"
	"strings"
)

type Provider string

const (
	ProviderNone    Provider = "none"
	ProviderAzureAD Provider = "azure_ad"
)

func (p Provider) Normalized() Provider {
	return normalizeProviderValue(p)
}

func (p Provider) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.marshalValue())
}

func (p *Provider) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = normalizeProviderValue(Provider(raw))
	return nil
}

func (p Provider) MarshalYAML() (any, error) {
	return p.marshalValue(), nil
}

func (p *Provider) UnmarshalYAML(unmarshal func(any) error) error {
	var raw string
	if err := unmarshal(&raw); err != nil {
		return err
	}
	*p = normalizeProviderValue(Provider(raw))
	return nil
}

func (p Provider) marshalValue() string {
	return string(p.Normalized())
}

func normalizeProviderValue(provider Provider) Provider {
	p := strings.ToLower(strings.TrimSpace(string(provider)))
	if p == "" {
		return ProviderNone
	}
	return Provider(p)
}
