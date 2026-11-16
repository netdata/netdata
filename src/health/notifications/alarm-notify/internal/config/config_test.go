// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sampleConfig struct {
	URL     string            `yaml:"url"`
	Number  *field.Integer    `yaml:"number,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Enabled *bool             `yaml:"enabled,omitempty"`
}
type sampleSender struct{ value sampleConfig }

func (*sampleSender) Send(context.Context, event.Event) error { return nil }

func sampleRegistry() Registry {
	return Registry{"sample": Factory(func(c sampleConfig) (notifier.Sender, error) { return &sampleSender{c}, nil })}
}

func TestStrictTypedConfiguration(t *testing.T) {
	const base = "version: 1\ndestinations:\n  first: &base\n    type: sample\n    url: https://example.com\n"
	for name, test := range map[string]struct {
		input string
		want  map[string]sampleConfig
		err   string
	}{
		"plain":             {input: base, want: map[string]sampleConfig{"first": {URL: "https://example.com"}}},
		"destination alias": {input: base + "  second: *base\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "second": {URL: "https://example.com"}}},
		"merge override":    {input: base + "  second:\n    <<: *base\n    url: https://example.org\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "second": {URL: "https://example.org"}}},
		"sequence merge":    {input: base + "  second:\n    <<: [*base]\n    number: 2\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "second": {URL: "https://example.com", Number: new(field.Integer(2))}}},
		"field key alias":   {input: "version: 1\ndestinations:\n  first:\n    type: sample\n    &field_name url: https://example.com\n  second:\n    type: sample\n    *field_name : https://example.org\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "second": {URL: "https://example.org"}}},
		"aliased foreign key named like valid field": {input: "version: 1\ndestinations:\n  first:\n    type: sample\n    headers: {&url synthetic-private-value: value}\n  second:\n    type: sample\n    *url : secret\n", err: "invalid YAML"},
		"scalar alias":                    {input: "version: 1\ndestinations:\n  first:\n    type: sample\n    url: &url https://example.com\n  second:\n    type: sample\n    url: *url\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "second": {URL: "https://example.com"}}},
		"null and false":                  {input: base + "    number: null\n    enabled: false\n", want: map[string]sampleConfig{"first": {URL: "https://example.com", Enabled: new(false)}}},
		"map keys are data":               {input: base + "    headers: {arbitrary: value}\n", want: map[string]sampleConfig{"first": {URL: "https://example.com", Headers: map[string]string{"arbitrary": "value"}}}},
		"root unknown":                    {input: base + "synthetic-private-value: secret\n", err: "invalid YAML"},
		"merge-tagged foreign field":      {input: base + "    !!merge synthetic-private-value: {}\n", err: "invalid YAML"},
		"merge-tagged null foreign field": {input: base + "    !!merge synthetic-private-value: null\n", err: "invalid YAML"},
		"merge-tagged root field":         {input: base + "!!merge synthetic-private-value: {}\n", err: "invalid YAML"},
		"merge-tagged destination name":   {input: base + "  !!merge synthetic-private-value:\n    type: sample\n    url: https://example.org\n", want: map[string]sampleConfig{"first": {URL: "https://example.com"}, "synthetic-private-value": {URL: "https://example.org"}}},
		"foreign empty":                   {input: base + "    synthetic-private-value: ''\n", err: "invalid YAML"},
		"foreign zero":                    {input: base + "    synthetic-private-value: 0\n", err: "invalid YAML"},
		"foreign null":                    {input: base + "    synthetic-private-value: null\n", err: "invalid YAML"},
		"duplicate":                       {input: base + "    url: secret\n", err: "invalid YAML"},
		"duplicate destination":           {input: base + "  first: *base\n", err: "invalid YAML"},
		"fraction":                        {input: base + "    number: 1.5\n", err: "invalid YAML"},
		"integer string":                  {input: base + "    number: '2'\n", err: "invalid YAML"},
		"alias integer string":            {input: base + "    headers: {a: &n '2'}\n    number: *n\n", err: "invalid YAML"},
		"unknown routing":                 {input: base + "routing: {synthetic-private-value: true}\n", err: "invalid YAML"},
		"unknown policy":                  {input: base + "routing: {policies: {first: {synthetic-private-value: true}}}\n", err: "invalid YAML"},
		"policy null flag":                {input: base + "routing: {policies: {first: {nowarn: null}}}\n", err: "invalid YAML"},
		"recursive alias":                 {input: "version: 1\ndestinations: &self {first: *self}\n", err: "not registered"},
		"extra document":                  {input: base + "---\n", err: "exactly one YAML"},
		"null destination":                {input: "version: 1\ndestinations: {first: null}\n", err: "not registered"},
	} {
		t.Run(name, func(t *testing.T) {
			plan, err := Read(strings.NewReader(test.input), sampleRegistry())
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, notifier.Plan{}, plan)
				return
			}
			require.NoError(t, err)
			got := map[string]sampleConfig{}
			for name, sender := range plan.Destinations {
				got[name] = sender.(*sampleSender).value
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestMergeRejectsForeignFields(t *testing.T) {
	type other struct {
		Other string `yaml:"other"`
	}
	registry := sampleRegistry()
	registry["other"] = Factory(func(c other) (notifier.Sender, error) { return &sampleSender{}, nil })
	input := `version: 1
destinations:
  other: &other
    type: other
    other: ""
  first:
    <<: *other
    type: sample
    url: https://example.com
`
	_, err := Read(strings.NewReader(input), registry)
	require.ErrorContains(t, err, "invalid YAML")
}
