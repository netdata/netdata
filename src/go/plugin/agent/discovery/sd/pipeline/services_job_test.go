// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceJobValues(t *testing.T) {
	for _, value := range []string{"", "on", "0123", "null", "x\nmodule: injected", "\n\n", " trailing\n\n", "\x00\x1f\u0085\u2028\u2029", "路由器", string([]byte{0xff, 0xfe})} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			input := map[string]any{
				"name":    value,
				"labels":  map[string]any{"zone": value},
				"port":    float64(80),
				"enabled": true,
			}
			for name, test := range map[string]struct {
				body string
				item any
			}{
				"pairs":    {body: "\n {{ job \"name\" .Item.name \"labels\" .Item.labels \"port\" .Item.port \"enabled\" .Item.enabled }} \n\n", item: input},
				"map":      {body: "{{ job .Item }}\n\n", item: input},
				"config":   {body: "{{ job .Item }}", item: confgroup.Config(input)},
				"YAML map": {body: "{{ job .Item }}", item: map[any]any{"name": value, "labels": map[any]any{"zone": value}, "port": 80, "enabled": true}},
			} {
				t.Run(name, func(t *testing.T) {
					engine, err := newServiceEngine(
						[]ServiceRuleConfig{{ID: "module", Match: "true", ConfigTemplate: test.body}},
					)
					require.NoError(t, err)
					got := engine.compose(&itemTarget{
						Item: test.item,
					})
					require.Equal(
						t,
						[]confgroup.Config{
							{
								"module":  "module",
								"name":    value,
								"labels":  map[any]any{"zone": value},
								"port":    80,
								"enabled": true,
							},
						},
						got,
					)
					assert.Equal(t, map[any]any{"zone": value}, got[0].Labels())
					assert.NotContains(t, input, "module", "module inference must not mutate the input")
				})
			}
		})
	}
}

func TestServiceJobSnapshots(t *testing.T) {
	body := `
{{ job .Item }}
{{ $_ := set .Item "name" "second" }}
{{ $_ := set .Item.labels "zone" "second-zone" }}
{{ job .Item }}
`
	engine, err := newServiceEngine([]ServiceRuleConfig{{ID: "module", Match: "true", ConfigTemplate: body}})
	require.NoError(t, err)
	input := map[string]any{"name": "first", "labels": map[string]any{"zone": "first-zone"}}
	got := engine.compose(&itemTarget{
		Item: input,
	})
	require.Equal(t, []confgroup.Config{
		{"module": "module", "name": "first", "labels": map[any]any{"zone": "first-zone"}},
		{"module": "module", "name": "second", "labels": map[any]any{"zone": "second-zone"}},
	}, got)
	input["labels"].(map[string]any)["zone"] = "later"
	assert.Equal(t, "first-zone", got[0].Labels()["zone"])
	assert.Equal(t, "second-zone", got[1].Labels()["zone"])
	got[0].Labels()["zone"] = "output-only"
	assert.Equal(t, "second-zone", got[1].Labels()["zone"])
}

func TestServiceJobRuleAtomicity(t *testing.T) {
	tests := map[string]struct {
		failure     string
		invalid     any
		definitions string
	}{
		"no arguments":       {failure: "{{ job }}"},
		"odd arguments":      {failure: `{{ job "name" "SYNTHETIC_SECRET" "dangling" }}`},
		"duplicate keys":     {failure: `{{ job "name" "one" "name" "two" }}`},
		"non-string key":     {failure: `{{ job 1 "value" }}`},
		"scalar":             {failure: `{{ job "scalar" }}`},
		"nil":                {failure: "{{ job .Item.invalid }}"},
		"nil string map":     {failure: "{{ job .Item.invalid }}", invalid: map[string]any(nil)},
		"nil YAML map":       {failure: "{{ job .Item.invalid }}", invalid: map[any]any(nil)},
		"nil config":         {failure: "{{ job .Item.invalid }}", invalid: confgroup.Config(nil)},
		"non-string map key": {failure: "{{ job .Item.invalid }}", invalid: map[any]any{1: "value"}},
		"list":               {failure: `{{ job (list (dict "name" "one")) }}`},
		"snapshot failure": {
			failure: `{{ job "password" "SYNTHETIC_SECRET" "unsupported" .Item.invalid }}`,
			invalid: func() {},
		},
		"missing field":      {failure: "{{ .Missing }}"},
		"template error":     {failure: `{{ fail "expected failure" }}`},
		"text after":         {failure: "name: unexpected"},
		"text before":        {failure: `name: unexpected {{ job "name" "also-discarded" }}`},
		"printed set result": {failure: `{{ set (dict) "key" "value" }}`},
		"nested error": {
			failure:     `{{ template "bad" . }}`,
			definitions: `{{ define "bad" }}{{ job "name" "nested" }}{{ job }}{{ end }}`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			body := test.definitions + `{{ job "name" "first" }}{{ if .Item.fail }}` + test.failure + `{{ end }}{{ job "name" "last" }}`
			engine, err := newServiceEngine([]ServiceRuleConfig{
				{ID: "module", Match: "true", ConfigTemplate: body},
				{ID: "fallback", Match: "true", ConfigTemplate: "name: legacy"},
			})
			require.NoError(t, err)
			var logs bytes.Buffer
			engine.Logger = logger.NewWithWriter(&logs)
			require.Equal(t, []confgroup.Config{{"module": "fallback", "name": "legacy"}}, engine.compose(&itemTarget{
				Item: map[string]any{"fail": true, "invalid": test.invalid},
			}))
			assert.Contains(t, logs.String(), "services[1]")
			assert.NotContains(t, logs.String(), "SYNTHETIC_SECRET")
			// A failed execution must not leak submitted jobs into the next target.
			require.Equal(t, []confgroup.Config{
				{"module": "module", "name": "first"},
				{"module": "module", "name": "last"},
				{"module": "fallback", "name": "legacy"},
			}, engine.compose(&itemTarget{
				Item: map[string]any{"fail": false},
			}))
		})
	}
}

func TestServiceJobNamedTemplates(t *testing.T) {
	body := `
{{ define "recurse" }}
  {{ job "name" (printf "job-%d" .) }}
  {{ if gt . 0 }}{{ template "recurse" (sub . 1) }}{{ end }}
{{ end }}
{{ template "recurse" 2 }}
`
	engine, err := newServiceEngine([]ServiceRuleConfig{{ID: "module", Match: "true", ConfigTemplate: body}})
	require.NoError(t, err)
	want := []confgroup.Config{
		{"module": "module", "name": "job-2"},
		{"module": "module", "name": "job-1"},
		{"module": "module", "name": "job-0"},
	}
	assert.Equal(t, want, engine.compose(&itemTarget{}))
	assert.Equal(t, want, engine.compose(&itemTarget{}))
}

func TestServiceJobOutputModes(t *testing.T) {
	for name, test := range map[string]struct {
		body string
		want []confgroup.Config
	}{
		"no calls":                {body: " \n{{ if false }}{{ job \"name\" \"unused\" }}{{ end }}\n "},
		"empty map":               {body: "{{ job (dict) }}\n", want: []confgroup.Config{{"module": "module"}}},
		"empty map mixed":         {body: "{{ job (dict) }}\nname: unexpected"},
		"mixed before first call": {body: "name: unexpected\n{{ job (dict) }}"},
		"legacy map":              {body: "name: legacy", want: []confgroup.Config{{"module": "module", "name": "legacy"}}},
		"legacy list":             {body: "- name: first\n- name: last", want: []confgroup.Config{{"module": "module", "name": "first"}, {"module": "module", "name": "last"}}},
		"legacy serializer":       {body: `{{ dict "name" "legacy" | toYaml }}`, want: []confgroup.Config{{"module": "module", "name": "legacy"}}},
		"explicit module":         {body: `{{ job "module" "other" "name" "value" }}`, want: []confgroup.Config{{"module": "other", "name": "value"}}},
	} {
		t.Run(name, func(t *testing.T) {
			engine, err := newServiceEngine(
				[]ServiceRuleConfig{{ID: "module", Match: "true", ConfigTemplate: test.body}},
			)
			require.NoError(t, err)
			assert.Equal(t, test.want, engine.compose(&itemTarget{}))
		})
	}
}

func TestServiceJobUnavailableInMatch(t *testing.T) {
	_, err := newServiceEngine([]ServiceRuleConfig{
		{ID: "first", Match: "true", ConfigTemplate: `{{ job "name" "first" }}`},
		{ID: "second", Match: `{{ job "name" "match" }}`, ConfigTemplate: "name: second"},
	})
	require.ErrorContains(t, err, "match")
	require.ErrorContains(t, err, `function "job" not defined`)
}

func TestServiceJobEngineIsolation(t *testing.T) {
	first, err := newServiceEngine(
		[]ServiceRuleConfig{{ID: "first", Match: "true", ConfigTemplate: `{{ job "name" .Item }}`}},
	)
	require.NoError(t, err)
	second, err := newServiceEngine(
		[]ServiceRuleConfig{{ID: "second", Match: "true", ConfigTemplate: `{{ job "name" .Item }}`}},
	)
	require.NoError(t, err)
	for _, name := range []string{"one", "two", "three"} {
		assert.Equal(t, []confgroup.Config{{"module": "first", "name": name}}, first.compose(&itemTarget{
			Item: name,
		}))
		assert.Equal(t, []confgroup.Config{{"module": "second", "name": name}}, second.compose(&itemTarget{
			Item: name,
		}))
	}
}
