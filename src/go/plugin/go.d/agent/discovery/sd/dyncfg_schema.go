// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	_ "embed"
)

// JSON schemas for each discoverer type.
// These are used by the Netdata UI to render configuration forms.

//go:embed "config_schema_net_listeners.json"
var schemaNetListeners string

//go:embed "config_schema_docker.json"
var schemaDocker string

//go:embed "config_schema_k8s.json"
var schemaK8s string

//go:embed "config_schema_snmp.json"
var schemaSNMP string

var discovererSchemas = map[string]string{
	DiscovererNetListeners: schemaNetListeners,
	DiscovererDocker:       schemaDocker,
	DiscovererK8s:          schemaK8s,
	DiscovererSNMP:         schemaSNMP,
}

// getDiscovererSchemaByType returns the JSON schema for a discoverer type.
// If the type is not found, returns a generic placeholder schema.
func getDiscovererSchemaByType(discovererType string) string {
	if schema, ok := discovererSchemas[discovererType]; ok {
		return schema
	}
	return schemaGeneric
}

const schemaGeneric = `{
  "jsonSchema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "title": "Service Discovery Pipeline Configuration",
    "properties": {
      "name": {
        "title": "Name",
        "type": "string",
        "description": "Pipeline name (must be unique)."
      }
    },
    "required": ["name"]
  },
  "uiSchema": {
    "uiOptions": {
      "fullPage": true
    }
  }
}`
