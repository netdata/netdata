// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import _ "embed"

//go:embed "config_schema_net_listeners.json"
var schemaNetListeners string

//go:embed "config_schema_docker.json"
var schemaDocker string

//go:embed "config_schema_k8s.json"
var schemaK8s string

//go:embed "config_schema_snmp.json"
var schemaSNMP string
