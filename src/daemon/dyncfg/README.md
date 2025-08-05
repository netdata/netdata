# Dynamic Configuration (DynCfg)

Dynamic Configuration (DynCfg) is a system in Netdata that enables both internal and external plugins/modules to expose their configurations dynamically to users through a unified interface. This document explains how DynCfg works and how to integrate your module with it.

## Overview

DynCfg provides a centralized mechanism for:

1. Registering configuration objects from any plugin or module
2. Providing a unified interface for users to view and modify these configurations
3. Persisting configurations between Netdata agent restarts
4. Validating configuration changes through the originating plugin/module
5. Standardizing configuration UI using JSON Schema

Key features:

- Plugins can expose multiple configuration objects
- Each configuration object has a unique ID
- The owning plugin validates configuration changes before being committed
- The DynCfg manager maintains the state of all dynamic configurations
- JSON Schema is used to define the structure of configuration objects
- The UI is based on adaptations of the react-jsonschema-form project

## Architecture

DynCfg consists of several API layers:

1. **Low-level API**: Core functionality used by both internal and external plugins
2. **High-level API**: Simplified API for internal plugins and modules
3. **External Plugin API**: Used by external plugins like go.d.plugin

## Integration Approaches

There are two main ways to integrate with DynCfg:

1. **High-level API for Internal Plugins**: Used by the health alerts system (documented here)
2. **External Plugin API**: Used by go.d.plugin (see [src/plugins.d/DYNCFG.md](/src/plugins.d/DYNCFG.md) for detailed documentation)

### Core Concepts

### Configuration ID Structure

The configuration ID is a crucial part of the DynCfg system. It serves as a unique identifier and determines how the configuration appears in the UI.

#### ID Format and Hierarchy

Configuration IDs typically follow a colon-separated hierarchical structure:

```
component:category:name
```

Where:

- **component**: The main module or plugin (e.g., "health", "go.d", "systemd-journal")
- **category**: Optional subcategory (e.g., "alert", "job")
- **name**: Specific configuration name

For example:

- `health:alert:prototype`: Health alert prototype template
- `health:alert:prototype:ram_usage`: Specific health alert prototype
- `go.d:nginx`: Nginx collector template
- `go.d:nginx:local_server`: Specific Nginx collector job
- `systemd-journal:monitored-directories`: Systemd journal configuration

#### Templates and Jobs

For templates and jobs, the ID follows a specific pattern:

1. **Template ID**: `component:template_name`
2. **Job ID**: `component:template_name:job_name`

The part before the last colon in a job ID must match an existing template ID.

#### UI Organization

The first component of the ID is used to organize configurations in the UI, creating separate tabs or sections. This allows for logical grouping of related configurations.

When choosing an ID, consider:

- UI organization (first component)
- Logical grouping (middle components)
- Uniqueness (complete ID)
- Readability for users

### Configuration Types

- **DYNCFG_TYPE_SINGLE**: A single configuration object
- **DYNCFG_TYPE_TEMPLATE**: A template for creating multiple job configurations
- **DYNCFG_TYPE_JOB**: A specific job configuration (derived from a template)

### Response Codes

DynCfg uses HTTP-like response codes to indicate the status of operations. These are crucial for both internal and external plugins to properly communicate with the DynCfg system:

#### Success Codes (2xx)

- **DYNCFG_RESP_RUNNING (200)**: Configuration was accepted and is currently running
- **DYNCFG_RESP_ACCEPTED (202)**: Configuration was accepted but not yet running
- **DYNCFG_RESP_ACCEPTED_DISABLED (298)**: Configuration was accepted but is currently disabled
- **DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED (299)**: Configuration was accepted but requires a restart to apply

#### Error Codes (4xx, 5xx)

Standard HTTP error codes are used, including:

- **HTTP_RESP_BAD_REQUEST (400)**: Invalid request or configuration
- **HTTP_RESP_NOT_FOUND (404)**: Requested configuration was not found
- **HTTP_RESP_INTERNAL_SERVER_ERROR (500)**: An internal error occurred
- **HTTP_RESP_NOT_IMPLEMENTED (501)**: The requested operation is not implemented

When implementing a callback function, always return the appropriate response code to indicate the status of the operation. The DynCfg system uses these codes to determine how to handle the configuration and what to display to the user.

### Source Types

- **DYNCFG_SOURCE_TYPE_INTERNAL**: Configuration defined internally within Netdata
- **DYNCFG_SOURCE_TYPE_DYNCFG**: Configuration created/modified through DynCfg
- **DYNCFG_SOURCE_TYPE_USER**: Configuration from user-provided files

#### Supported Commands

- **DYNCFG_CMD_SCHEMA**: Get JSON schema for the configuration
- **DYNCFG_CMD_GET**: Get the current configuration
- **DYNCFG_CMD_UPDATE**: Update the configuration
- **DYNCFG_CMD_DISABLE**: Disable the configuration
- **DYNCFG_CMD_ENABLE**: Enable the configuration
- **DYNCFG_CMD_ADD**: Add a new job (for templates only)
- **DYNCFG_CMD_REMOVE**: Remove a job (for DYNCFG_SOURCE_TYPE_DYNCFG jobs only)
- **DYNCFG_CMD_TEST**: Test a configuration without applying it
- **DYNCFG_CMD_RESTART**: Restart the configuration
- **DYNCFG_CMD_USERCONFIG**: Get the configuration in a user-friendly format (used for conf files)

## Implementing DynCfg for Internal Plugins

Here's how to implement DynCfg for an internal plugin/module:

### 1. Register Your Configuration

For internal plugins, the Netdata daemon already handles initialization and shutdown of the DynCfg system. You don't need to call `dyncfg_init()` or `dyncfg_shutdown()` in your plugin code.

Register your configurations when your plugin initializes:

```c
bool dyncfg_add(
    RRDHOST *host,               // The host this configuration belongs to (localhost for global configs)
    const char *id,              // Unique ID for this configuration
    const char *path,            // Path for UI organization
    DYNCFG_STATUS status,        // Initial status (ACCEPTED, DISABLED, etc.)
    DYNCFG_TYPE type,            // SINGLE, TEMPLATE, or JOB
    DYNCFG_SOURCE_TYPE source_type, // INTERNAL, DYNCFG, USER
    const char *source,          // Source identifier (e.g., "internal")
    DYNCFG_CMDS cmds,            // Supported commands (bitwise OR of DYNCFG_CMD_* values)
    HTTP_ACCESS view_access,     // Access permissions for viewing
    HTTP_ACCESS edit_access,     // Access permissions for editing
    dyncfg_cb_t cb,              // Callback function for handling commands
    void *data                   // User data passed to the callback
);
```

Example (from health_dyncfg.c):

```c
dyncfg_add(
    localhost,
    DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX, 
    "/health/alerts/prototypes",
    DYNCFG_STATUS_ACCEPTED, 
    DYNCFG_TYPE_TEMPLATE,
    DYNCFG_SOURCE_TYPE_INTERNAL, 
    "internal",
    DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_USERCONFIG,
    HTTP_ACCESS_NONE,
    HTTP_ACCESS_NONE,
    dyncfg_health_cb, 
    NULL
);
```

### 3. Implement a Callback Function

```c
int your_dyncfg_callback(
    const char *transaction,  // Transaction ID
    const char *id,           // Configuration ID
    DYNCFG_CMDS cmd,          // Command being executed
    const char *add_name,     // For ADD: the name to add
    BUFFER *payload,          // Command payload
    usec_t *stop_monotonic_ut, // Timeout info
    bool *cancelled,          // Whether the operation was cancelled
    BUFFER *result,           // Buffer to write results to
    HTTP_ACCESS access,       // Access level of the user
    const char *source,       // Configuration source
    void *data                // User data passed during registration
) {
    // Handle the command
    switch(cmd) {
        case DYNCFG_CMD_SCHEMA:
            // Return JSON schema
            buffer_json_initialize(result, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
            // Add your schema here
            buffer_json_finalize(result);
            return HTTP_RESP_OK;
            
        case DYNCFG_CMD_GET:
            // Return current configuration
            buffer_json_initialize(result, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
            // Add your configuration here
            buffer_json_finalize(result);
            return HTTP_RESP_OK;
            
        case DYNCFG_CMD_UPDATE:
            // Process configuration update
            // If update successful
            return DYNCFG_RESP_RUNNING; // or DYNCFG_RESP_ACCEPTED if not running yet
            
            // If restart is required
            // return DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED;
            
            // If validation fails
            // return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "your error message");
            
        case DYNCFG_CMD_DISABLE:
            // Handle disabling the configuration
            return DYNCFG_RESP_ACCEPTED_DISABLED;
            
        case DYNCFG_CMD_ENABLE:
            // Handle enabling the configuration
            return DYNCFG_RESP_RUNNING; // or DYNCFG_RESP_ACCEPTED if not running yet
            
        // Handle other commands
        
        default:
            // Unsupported command
            return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "unsupported command");
    }
}
```

### 4. Configuration Lifecycle Management

Unregister configurations only when they’re no longer conceptually relevant (such as when a feature becomes unavailable), not during plugin or module shutdown:

```c
dyncfg_del(host, id);  // Remove a specific configuration that no longer applies
```

When a plugin or module exits, the Netdata Functions manager automatically stops accepting requests for its functions. The configurations will remain in the DynCfg system and will become active again when the plugin restarts.

The Netdata daemon also handles shutting down the entire DynCfg system, so you don't need to call `dyncfg_shutdown()` in your plugin code.

## Implementing DynCfg for External Plugins

External plugins like go.d.plugin use a different approach based on the plugins.d protocol. For detailed information on implementing DynCfg for external plugins, please refer to the [External Plugins DynCfg documentation](/src/plugins.d/DYNCFG.md).

This documentation covers:

- How to register configurations using the CONFIG command
- How to respond to configuration commands
- How to update configuration status
- How to delete configurations
- Detailed examples and best practices

## JSON Schema for Configuration UI

The UI for modifying configurations is generated from JSON Schema. Netdata supports two ways to provide schema definitions:

### 1. Static Schema Files

Netdata first attempts to load schema files from the disk before calling the plugin or module. Schema files should be placed in:

- `CONFIG_DIR/schema.d/` (user-provided schemas, typically `/etc/netdata/schema.d/`)
- `LIBCONFIG_DIR/schema.d/` (stock schemas, typically `/usr/lib/netdata/conf.d/schema.d/`)

Schema files should be named after the configuration ID with `.json` extension, for example:

- `health:alert:prototype.json`
- `go.d:nginx.json`

### 2. Dynamic Schema Generation

If no static schema file is found, Netdata will call the plugin or module with the `DYNCFG_CMD_SCHEMA` command:

1. For internal plugins, the callback function should return a JSON Schema document
2. For external plugins, the plugin should respond to the schema command with a JSON Schema document

This gives you flexibility to either:

- Use static schema files for simple, fixed configurations
- Generate schemas dynamically for more complex configurations that may change based on runtime conditions

Example JSON Schema:

```json
{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "format": "uri",
      "title": "Server URL",
      "description": "The URL of the server to connect to"
    },
    "timeout": {
      "type": "integer",
      "minimum": 1,
      "maximum": 60,
      "title": "Timeout",
      "description": "Connection timeout in seconds"
    },
    "auth": {
      "type": "object",
      "title": "Authentication",
      "properties": {
        "username": {
          "type": "string",
          "title": "Username"
        },
        "password": {
          "type": "string",
          "format": "password",
          "title": "Password"
        }
      }
    }
  },
  "required": [
    "url"
  ]
}
```

## Action Behavior by Configuration Type

Different actions behave differently depending on the configuration type. The following table explains what happens when each action is applied to different configuration types:

| Action         | SINGLE                               | TEMPLATE                                         | JOB                                                   |
|----------------|--------------------------------------|--------------------------------------------------|-------------------------------------------------------|
| **SCHEMA**     | Returns schema from file or plugin   | Returns schema from file or plugin               | Uses template's schema                                |
| **GET**        | Returns current configuration        | Not supported (templates don't maintain data)    | Returns current configuration                         |
| **UPDATE**     | Updates single configuration         | Not supported (templates don't maintain data)    | Updates job configuration                             |
| **ADD**        | Not supported                        | Creates a new job based on template              | Not supported                                         |
| **REMOVE**     | Not supported                        | Not supported                                    | Removes job (only for DYNCFG_SOURCE_TYPE_DYNCFG jobs) |
| **ENABLE**     | Enables single configuration         | Enables template AND all its jobs                | Enables job (fails if its template is disabled)       |
| **DISABLE**    | Disables single configuration        | Disables template AND all its jobs               | Disables job                                          |
| **RESTART**    | Sends restart command to plugin      | Restarts all template's jobs                     | Restarts specific job                                 |
| **TEST**       | Tests configuration without applying | Tests a new job configuration                    | Tests updated job configuration                       |
| **USERCONFIG** | Returns user-friendly configuration  | Returns template for user-friendly configuration | Returns user-friendly configuration                   |

**Important Notes:**

- When a template is disabled, all its jobs are automatically disabled regardless of their individual settings
- Jobs can’t be enabled if their parent template is disabled
- Template schemas are used for all jobs created from that template
- The REMOVE action is only available for jobs created via DynCfg (with source type DYNCFG_SOURCE_TYPE_DYNCFG)
- RESTART on a template recursively restarts all jobs associated with that template

## API Access

The DynCfg system exposes configurations via the Netdata API for management:

### Tree API

Netdata provides a tree API that returns the entire DynCfg tree or a specific subpath:

```
/api/v3/config?action=tree&path=/collectors
```

Parameters:

- `action`: Set to "tree" to get the configuration tree
- `path`: The configuration path to start from (e.g., "/collectors", "/health/alerts")
- `id` (optional): Return only a specific configuration ID

This returns a JSON structure of all configurations, organized by path, with their status, commands, access controls, and other metadata.

### Configuration Actions API

Individual configurations can be managed via:

```
/api/v3/config?action=<command>&id=<configuration_id>&name=<name>
```

Where:

- `action`: One of the supported commands (get, update, add, remove, etc.)
- `id`: The configuration ID to act upon
- `name`: Required for add/test actions (new job name)

The API automatically routes commands to the appropriate plugin or module that registered the configuration.

## Best Practices

1. **Use Clear IDs**: Configuration IDs should be clear and hierarchical (e.g., "mymodule:config1")
2. **Choose Appropriate Paths**: The path parameter controls UI organization - select logical paths for easy navigation
3. **Validate Thoroughly**: Always validate configuration changes before accepting them
4. **Provide Helpful Errors**: Return detailed error messages when rejections occur
5. **Document Your Schema**: Include descriptions for all properties in your JSON Schema
6. **Clean Up**: Remove configurations when modules are unloaded or no longer needed
7. **Security**: Set appropriate view/edit access controls for sensitive configurations

## Example Implementations

### Health Alerts System (Internal Plugin)

Health alerts use DynCfg to manage alert definitions. Key files:

- `src/health/health_dyncfg.c`: Implements DynCfg integration for health alerts
- Handles alert prototype templates and individual alert configurations

The health module uses the high-level API with IDs like:

- `health:alert:prototype` for the alert template
- `health:alert:prototype:ram_usage` for specific alert prototypes

It supports multiple configuration objects, validation of alert definitions, and conversion between different configuration formats (JSON and traditional Netdata health configuration syntax).

### journal-viewer-plugin (External Plugin)

The journal-viewer-plugin is an external plugin written in Rust that provides systemd journal log viewing and analysis:

- **Location**: `src/crates/netdata-log-viewer/journal-viewer-plugin/`
- Provides systemd journal log querying and visualization capabilities
- Implements the Netdata plugin protocol for communication with the agent

### go.d.plugin (External Plugin)

go.d.plugin uses DynCfg to manage job configurations. It:

1. Registers configurations through the plugins.d protocol
2. Generates dynamic JSON Schema based on Go struct tags
3. Handles configuration updates for collecting jobs

It uses IDs like:

- `go.d:nginx` for the Nginx collector template
- `go.d:nginx:local_server` for a specific Nginx collector job

go.d.plugin demonstrates how external plugins can leverage DynCfg to provide dynamic configuration capabilities through the plugins.d protocol.

## Debugging Tips

1. Set the environment variable `NETDATA_DEBUG_DYNCFG=1` to enable debug logging for DynCfg
2. Check `/var/lib/netdata/config/` for persisted configuration files
3. Inspect configurations via the API: `/api/v3/config?id=<your-config-id>`
