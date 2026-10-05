// SPDX-License-Identifier: GPL-3.0-or-later

package control

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDynCfgFormName(t *testing.T) {
	for _, kind := range []string{"hostmetrics", "filelogs"} {
		t.Run(kind, func(t *testing.T) {
			// Compile the serialized schema as the UI receives it.
			raw, err := json.Marshal(schema(kind))
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("poc.json", document["jsonSchema"]); err != nil {
				t.Fatal(err)
			}
			compiled, err := compiler.Compile("poc.json")
			if err != nil {
				t.Fatal(err)
			}
			form := map[string]any{"service_name": "form-service"}
			if kind == "hostmetrics" {
				form["interval"] = "2s"
			} else {
				form["paths"] = []any{"/tmp/example.log"}
			}
			for _, name := range []any{nil, "", "form-job"} {
				if name != nil {
					form["name"] = name
				}
				if err := compiled.Validate(form); err != nil {
					t.Errorf("UI form with name %v rejected by schema: %v", name, err)
				}
				payload, _ := json.Marshal(form)
				if _, err := parseConfig(kind, payload); err != nil {
					t.Errorf("UI form with name %v rejected by decoder: %v", name, err)
				}
			}
			for _, invalid := range []any{42, nil, map[string]any{}} {
				form["name"] = invalid
				payload, _ := json.Marshal(form)
				if compiled.Validate(form) == nil {
					t.Errorf("schema accepted malformed name: %s", payload)
				}
				if _, err := parseConfig(kind, payload); err == nil {
					t.Errorf("decoder accepted malformed name: %s", payload)
				}
			}
			delete(form, "name")
			form["unknown_setting"] = true
			payload, _ := json.Marshal(form)
			if compiled.Validate(form) == nil {
				t.Fatal("schema accepted unknown receiver setting")
			}
			if _, err := parseConfig(kind, payload); err == nil {
				t.Fatal("decoder accepted unknown receiver setting")
			}
		})
	}
}

func TestDynCfgNameDoesNotChangeIdentityOrReload(t *testing.T) {
	for _, kind := range []string{"hostmetrics", "filelogs"} {
		t.Run(kind, func(t *testing.T) {
			c, out := newTestController(t)
			template := Prefix + kind
			id := template + ":command-name"
			fields := `"interval":"2s"`
			if kind == "filelogs" {
				fields = `"paths":["/tmp/example.log"]`
			}
			payload := fmt.Sprintf(`{"name":"form-name","service_name":"service",%s}`, fields)
			if code := command(t, c, out, template+" test command-name", payload); code != 200 {
				t.Fatalf("test rejected UI metadata: %d", code)
			}
			if code := command(t, c, out, template+" add command-name", payload); code != 202 {
				t.Fatalf("add rejected UI metadata: %d", code)
			}
			if c.jobs[id] == nil || c.jobs[template+":form-name"] != nil {
				t.Fatal("payload name overrode command-owned identity")
			}
			command(t, c, out, id+" enable", "")
			_, changed, _, _ := c.Snapshot()
			payload = fmt.Sprintf(`{"name":"different-form-name","service_name":"service",%s}`, fields)
			if code := command(t, c, out, id+" update", payload); code != 202 {
				t.Fatalf("metadata-only update: %d", code)
			}
			select {
			case <-changed:
				t.Fatal("metadata-only update reloaded pipelines")
			default:
			}
			raw, _ := json.Marshal(c.jobs[id].Config)
			var got map[string]any
			_ = json.Unmarshal(raw, &got)
			if _, exists := got["name"]; exists {
				t.Fatal("form metadata leaked into canonical configuration")
			}
			if code := command(t, c, out, template+" add invalid.name", payload); code != 400 {
				t.Fatalf("payload name bypassed command name validation: %d", code)
			}
		})
	}
}
