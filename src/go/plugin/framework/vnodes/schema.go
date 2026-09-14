// SPDX-License-Identifier: GPL-3.0-or-later

package vnodes

import "encoding/json"

// ConfigSchemaFor omits acquisition controls from plugins without an adapter.
func ConfigSchemaFor(snmpSupported bool) string {
	if snmpSupported {
		return ConfigSchema
	}
	return staticConfigSchema
}

var staticConfigSchema = func() string {
	var doc map[string]map[string]any
	if err := json.Unmarshal([]byte(ConfigSchema), &doc); err != nil {
		panic(err)
	}
	schema, ui := doc["jsonSchema"], doc["uiSchema"]
	delete(schema["properties"].(map[string]any), "mode")
	delete(schema, "dependencies")
	delete(schema, "allOf")
	schema["required"] = []string{"guid"}
	delete(ui, "mode")
	delete(ui, "mode_snmp")
	ui["ui:order"] = []string{"hostname", "guid", "labels", "stale_after"}
	properties := schema["properties"].(map[string]any)
	properties["hostname"].(map[string]any)["description"] = "Display hostname. Omit to use the resource name."
	properties["guid"].(map[string]any)["description"] = "Unique node UUID. Changing this value creates a new node; existing history stays with the old UUID."
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return string(raw)
}()
