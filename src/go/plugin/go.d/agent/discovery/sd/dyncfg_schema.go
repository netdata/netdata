// SPDX-License-Identifier: GPL-3.0-or-later

package sd

// JSON schemas for each discoverer type.
// These are used by the Netdata UI to render configuration forms.

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
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Service Discovery Pipeline Configuration",
  "properties": {
    "name": {
      "type": "string",
      "description": "Pipeline name (must be unique)"
    }
  },
  "required": ["name"]
}`

const schemaNetListeners = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Net Listeners Service Discovery",
  "description": "Discovers services by scanning local listening network ports",
  "properties": {
    "name": {
      "type": "string",
      "description": "Pipeline name (must be unique)",
      "minLength": 1
    },
    "interval": {
      "type": "string",
      "description": "How often to scan for listeners (default: 2m)",
      "pattern": "^[0-9]+(s|m|h)$",
      "examples": ["30s", "2m", "5m"]
    },
    "timeout": {
      "type": "string",
      "description": "Timeout for local listener discovery (default: 5s)",
      "pattern": "^[0-9]+(s|m)$",
      "examples": ["5s", "10s"]
    },
    "tags": {
      "type": "string",
      "description": "Tags to apply to discovered targets (space-separated)"
    },
    "services": {
      "type": "array",
      "description": "Service matching rules",
      "items": {
        "$ref": "#/$defs/serviceRule"
      }
    }
  },
  "required": ["name"],
  "$defs": {
    "serviceRule": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "description": "Rule identifier (for logging/diagnostics)"
        },
        "match": {
          "type": "string",
          "description": "Expression to match targets (e.g., '{{.Port}} == 80')"
        },
        "config_template": {
          "type": "string",
          "description": "YAML template for collector config generation"
        }
      },
      "required": ["id", "match"]
    }
  }
}`

const schemaDocker = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Docker Service Discovery",
  "description": "Discovers services running in Docker containers",
  "properties": {
    "name": {
      "type": "string",
      "description": "Pipeline name (must be unique)",
      "minLength": 1
    },
    "address": {
      "type": "string",
      "description": "Docker daemon address (default: unix:///var/run/docker.sock)",
      "examples": ["unix:///var/run/docker.sock", "tcp://localhost:2375"]
    },
    "timeout": {
      "type": "string",
      "description": "Timeout for Docker API calls (default: 2s)",
      "pattern": "^[0-9]+(s|m)$",
      "examples": ["2s", "5s"]
    },
    "tags": {
      "type": "string",
      "description": "Tags to apply to discovered targets (space-separated)"
    },
    "services": {
      "type": "array",
      "description": "Service matching rules",
      "items": {
        "$ref": "#/$defs/serviceRule"
      }
    }
  },
  "required": ["name"],
  "$defs": {
    "serviceRule": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "description": "Rule identifier (for logging/diagnostics)"
        },
        "match": {
          "type": "string",
          "description": "Expression to match containers (e.g., '{{.Image}} == \"nginx\"')"
        },
        "config_template": {
          "type": "string",
          "description": "YAML template for collector config generation"
        }
      },
      "required": ["id", "match"]
    }
  }
}`

const schemaK8s = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "Kubernetes Service Discovery",
  "description": "Discovers services running in Kubernetes cluster",
  "properties": {
    "name": {
      "type": "string",
      "description": "Pipeline name (must be unique)",
      "minLength": 1
    },
    "role": {
      "type": "string",
      "description": "Kubernetes resource role to discover",
      "enum": ["pod", "service"],
      "default": "pod"
    },
    "namespaces": {
      "type": "array",
      "description": "Namespaces to watch (empty = all namespaces)",
      "items": {
        "type": "string"
      }
    },
    "selector": {
      "type": "object",
      "description": "Label and field selectors for filtering resources",
      "properties": {
        "label": {
          "type": "string",
          "description": "Label selector (e.g., 'app=nginx')"
        },
        "field": {
          "type": "string",
          "description": "Field selector (e.g., 'metadata.name=my-pod')"
        }
      }
    },
    "pod": {
      "type": "object",
      "description": "Pod-specific options",
      "properties": {
        "local_mode": {
          "type": "boolean",
          "description": "Only discover pods on the same node as the agent",
          "default": false
        }
      }
    },
    "tags": {
      "type": "string",
      "description": "Tags to apply to discovered targets (space-separated)"
    },
    "services": {
      "type": "array",
      "description": "Service matching rules",
      "items": {
        "$ref": "#/$defs/serviceRule"
      }
    }
  },
  "required": ["name", "role", "tags"],
  "$defs": {
    "serviceRule": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "description": "Rule identifier (for logging/diagnostics)"
        },
        "match": {
          "type": "string",
          "description": "Expression to match resources"
        },
        "config_template": {
          "type": "string",
          "description": "YAML template for collector config generation"
        }
      },
      "required": ["id", "match"]
    }
  }
}`

const schemaSNMP = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "title": "SNMP Device Discovery",
  "description": "Discovers SNMP devices by scanning network subnets",
  "properties": {
    "name": {
      "type": "string",
      "description": "Pipeline name (must be unique)",
      "minLength": 1
    },
    "rescan_interval": {
      "type": "string",
      "description": "How often to scan networks for devices (default: 30m)",
      "pattern": "^[0-9]+(s|m|h)$",
      "examples": ["30m", "1h"]
    },
    "timeout": {
      "type": "string",
      "description": "Timeout for SNMP device responses (default: 1s)",
      "pattern": "^[0-9]+(s|m)$",
      "examples": ["1s", "5s"]
    },
    "device_cache_ttl": {
      "type": "string",
      "description": "How long to cache discovery results (default: 12h)",
      "pattern": "^[0-9]+(s|m|h)$",
      "examples": ["12h", "24h"]
    },
    "parallel_scans_per_network": {
      "type": "integer",
      "description": "Concurrent IPs to scan per subnet (default: 32)",
      "minimum": 1,
      "maximum": 256,
      "default": 32
    },
    "credentials": {
      "type": "array",
      "description": "SNMP credentials for authentication",
      "items": {
        "$ref": "#/$defs/credential"
      }
    },
    "networks": {
      "type": "array",
      "description": "Network subnets to scan",
      "items": {
        "$ref": "#/$defs/network"
      }
    },
    "services": {
      "type": "array",
      "description": "Service matching rules",
      "items": {
        "$ref": "#/$defs/serviceRule"
      }
    }
  },
  "required": ["name", "credentials", "networks"],
  "$defs": {
    "credential": {
      "type": "object",
      "properties": {
        "name": {
          "type": "string",
          "description": "Credential identifier (referenced by networks)"
        },
        "version": {
          "type": "string",
          "description": "SNMP version",
          "enum": ["1", "2c", "3"],
          "default": "2c"
        },
        "community": {
          "type": "string",
          "description": "Community string (for SNMP v1/v2c)",
          "default": "public"
        },
        "username": {
          "type": "string",
          "description": "Username (for SNMPv3)"
        },
        "security_level": {
          "type": "string",
          "description": "Security level (for SNMPv3)",
          "enum": ["noAuthNoPriv", "authNoPriv", "authPriv"]
        },
        "auth_protocol": {
          "type": "string",
          "description": "Authentication protocol (for SNMPv3)",
          "enum": ["md5", "sha", "sha224", "sha256", "sha384", "sha512"]
        },
        "auth_password": {
          "type": "string",
          "description": "Authentication passphrase (for SNMPv3)"
        },
        "priv_protocol": {
          "type": "string",
          "description": "Privacy protocol (for SNMPv3)",
          "enum": ["des", "aes", "aes192", "aes256", "aes192C", "aes256C"]
        },
        "priv_password": {
          "type": "string",
          "description": "Privacy passphrase (for SNMPv3)"
        }
      },
      "required": ["name", "version"]
    },
    "network": {
      "type": "object",
      "properties": {
        "subnet": {
          "type": "string",
          "description": "IP range to scan (e.g., '192.168.1.0/24', '10.0.0.1-10.0.0.50')"
        },
        "credential": {
          "type": "string",
          "description": "Name of credential to use for this network"
        }
      },
      "required": ["subnet", "credential"]
    },
    "serviceRule": {
      "type": "object",
      "properties": {
        "id": {
          "type": "string",
          "description": "Rule identifier (for logging/diagnostics)"
        },
        "match": {
          "type": "string",
          "description": "Expression to match devices"
        },
        "config_template": {
          "type": "string",
          "description": "YAML template for collector config generation"
        }
      },
      "required": ["id", "match"]
    }
  }
}`
