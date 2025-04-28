# Dynamic Configuration for External Plugins

External plugins in Netdata can expose dynamic configuration capabilities through the DynCfg system. This document explains how to implement DynCfg in external plugins using the plugins.d protocol.

## Overview

The DynCfg system allows external plugins to:

1. Register configurable entities (both single configurations and templates for creating jobs)
2. Receive configuration commands from users
3. Validate and apply configurations
4. Persist configurations between Netdata agent restarts

## Protocol Commands

DynCfg for external plugins uses the following plugins.d protocol commands:

1. `CONFIG`: Sent from the plugin to Netdata to register, update status, or delete configurations
2. `FUNCTION`/`FUNCTION_PAYLOAD_BEGIN`: Received by the plugin to handle configuration commands
3. `FUNCTION_RESULT_BEGIN`: Sent from the plugin to respond to commands

## Implementing DynCfg in External Plugins

### 1. Register a Configuration

To register a configuration, the plugin sends the CONFIG command:

```
CONFIG <id> CREATE <status> <type> <path> <source_type> <source> <cmds> <view_access> <edit_access>
```

Where:

- `id` is a unique identifier for the configurable entity (e.g., "go.d:nginx")
- `status` can be:
    - `accepted`: Configuration is accepted but not running
    - `running`: Configuration is accepted and running
    - `failed`: Plugin fails to run the configuration
    - `incomplete`: Plugin needs additional settings
    - `disabled`: Configuration is disabled by a user
- `type` can be:
    - `single`: A single configuration object (not addable or removable by users)
    - `template`: A template for creating multiple job configurations
    - `job`: A specific job configuration (derived from a template)
- `path` is the UI organization path (usually "/collectors") that determines where in the configuration tree the item will appear in the UI. This is separate from the ID and controls the hierarchical navigation structure.
- `source_type` can be:
    - `internal`: Based on internal code settings
    - `stock`: Default configurations
    - `user`: User configurations via a file
    - `dyncfg`: Configuration received via this mechanism
    - `discovered`: Dynamically discovered by the plugin
- `source` provides more details about the exact source
- `cmds` is a space or pipe (|) separated list of supported commands:
    - `schema`: Get JSON schema for the configuration
    - `get`: Get current configuration values
    - `update`: Receive configuration updates
    - `add`: Receive job creation commands (templates only)
    - `remove`: Remove a configuration (jobs only)
    - `enable`/`disable`: Enable or disable the configuration
    - `test`: Test a configuration without applying it
    - `restart`: Restart the configuration
    - `userconfig`: Get user-friendly configuration format
- `view_access` and `edit_access` are permission bitmaps (use 0 for default permissions)

Example:

```
CONFIG go.d:nginx CREATE accepted template /collectors internal internal schema|add|enable|disable 0 0
CONFIG go.d:nginx:local_server CREATE running job /collectors dyncfg user schema|get|update|remove|enable|disable|restart 0 0
```

### 2. Respond to Configuration Commands

The plugin receives configuration commands from Netdata as plugin functions. These come in two forms:

#### Without Payload:

```
FUNCTION <transaction_id> <timeout_ms> "config <id> <command>" "<http_access>" "<source>"
```

Used for commands like: `schema`, `get`, `remove`, `enable`, `disable`, `restart`

Example:

```
FUNCTION abcd1234 60 "config go.d:nginx:local_server get" "member" "netdata-cli"
```

#### With Payload:

```
FUNCTION_PAYLOAD_BEGIN <transaction_id> <timeout_ms> "config <id> <command>" "<http_access>" "<source>" "<content_type>"
<payload_data>
FUNCTION_PAYLOAD_END
```

Used for commands like: `update`, `add`, `test` that require additional data.

Example:

```
FUNCTION_PAYLOAD_BEGIN abcd1234 60 "config go.d:nginx:local_server update" "member" "netdata-cli" "application/json"
{
  "url": "http://localhost:80/stub_status",
  "timeout": 5,
  "update_every": 10
}
FUNCTION_PAYLOAD_END
```

### 3. Process Commands and Respond

After receiving a command, the plugin should process it and respond with a function result:

```
FUNCTION_RESULT_BEGIN <transaction_id> <http_status_code> <content_type> <expiration>
<result_data>
FUNCTION_RESULT_END
```

Where:

- `transaction_id` is the same ID received in the original command
- `http_status_code` is the standard HTTP response code:
    - `200`: Success (DYNCFG_RESP_RUNNING) - Configuration accepted and running
    - `202`: Accepted (DYNCFG_RESP_ACCEPTED) - Configuration accepted but not running yet
    - `298`: Accepted but disabled (DYNCFG_RESP_ACCEPTED_DISABLED)
    - `299`: Accepted but restart required (DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED)
    - `400`: Bad request - Invalid configuration
    - `404`: Not found - Configuration not found
    - `500`: Internal server error
- `content_type` is typically "application/json"
- `expiration` is the absolute timestamp (unix epoch) for result expiration

The result data depends on the command:

- `schema`: Return JSON Schema document
- `get`: Return current configuration values
- Other commands: Return a success or error message

Success response example:

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "status": 200,
  "message": "Configuration updated successfully"
}
FUNCTION_RESULT_END
```

Error response example:

```
FUNCTION_RESULT_BEGIN abcd1234 400 application/json 0
{
  "status": 400,
  "error_message": "Invalid URL format"
}
FUNCTION_RESULT_END
```

### 4. Update Configuration Status

To update the status of a configuration after it's been created:

```
CONFIG <id> STATUS <new_status>
```

Example:

```
CONFIG go.d:nginx:local_server STATUS running
```

This is useful when a configuration transitions from "accepted" to "running" or "failed" after being tested.

### 5. Delete a Configuration

When a configuration is no longer available (e.g., the monitored service is removed):

```
CONFIG <id> DELETE
```

Example:

```
CONFIG go.d:nginx:local_server DELETE
```

## JSON Schema for Configuration UI

DynCfg uses JSON Schema to define the structure of configuration objects, which is used to generate the UI.

### Static Schema Files (Optional)

Before calling the plugin, Netdata will first attempt to find a static schema file. You can provide static schema files in:

- `CONFIG_DIR/schema.d/` (user-provided schemas, typically `/etc/netdata/schema.d/`)
- `LIBCONFIG_DIR/schema.d/` (stock schemas, typically `/usr/lib/netdata/conf.d/schema.d/`)

Schema files should be named after the configuration ID with `.json` extension:

```
/etc/netdata/schema.d/go.d:nginx.json
```

This approach is useful for stable schemas that don't change frequently.

### Dynamic Schema Generation

If no static schema file is found, Netdata will send a `schema` command to the plugin. When handling a `schema` request, the plugin should return a JSON Schema document:

```json
{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "format": "uri",
      "title": "Server URL",
      "description": "The URL of the Nginx stub_status endpoint"
    },
    "timeout": {
      "type": "integer",
      "minimum": 1,
      "maximum": 60,
      "title": "Timeout",
      "description": "Connection timeout in seconds"
    },
    "update_every": {
      "type": "integer",
      "minimum": 1,
      "title": "Update Every",
      "description": "Data collection frequency in seconds"
    }
  },
  "required": [
    "url"
  ]
}
```

For templates, the schema will be used when users add new jobs based on the template.

## Action Behavior Reference

When implementing DynCfg in your external plugin, be aware of how actions should behave based on the configuration type:

| Action         | TEMPLATE                                | JOB                                     |
|----------------|-----------------------------------------|-----------------------------------------|
| **SCHEMA**     | Return schema for creating new jobs     | Use template's schema                   |
| **GET**        | Not applicable                          | Return current configuration            |
| **UPDATE**     | Not applicable                          | Update configuration and apply if valid |
| **ADD**        | Create new job from template            | Not applicable                          |
| **REMOVE**     | Not supported                           | Remove job (only for user-created jobs) |
| **ENABLE**     | Enable template and all its jobs        | Enable specific job                     |
| **DISABLE**    | Disable template and all its jobs       | Disable specific job                    |
| **RESTART**    | Restart all jobs based on template      | Restart specific job                    |
| **TEST**       | Test a potential job configuration      | Test configuration changes              |
| **USERCONFIG** | Return template in user-friendly format | Return job in user-friendly format      |

**Important Implementation Notes:**

- When a template is disabled, send DISABLE commands to all jobs of that template
- Reject ENABLE commands for jobs if their template is disabled
- For job SCHEMA requests, return the same schema as the template
- REMOVE should only work on dynamically added jobs, not ones from static configurations
- Return appropriate response codes to indicate the status (running, accepted, disabled)

## External Plugin Examples

### C-based External Plugin (systemd-journal.plugin)

The systemd-journal.plugin is a C-based external plugin that uses DynCfg to manage journal directory configurations. It implements a SINGLE configuration type to manage the list of journald directories to monitor:

```c
// Register the configuration
functions_evloop_dyncfg_add(
    wg,
    "systemd-journal:monitored-directories",  // ID
    "/logs/systemd-journal",                  // UI Path
    DYNCFG_STATUS_RUNNING,                    // Status
    DYNCFG_TYPE_SINGLE,                       // Type - single configuration
    DYNCFG_SOURCE_TYPE_INTERNAL,              // Source type
    "internal",                               // Source
    DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET | DYNCFG_CMD_UPDATE,  // Supported commands
    HTTP_ACCESS_NONE,                         // View permissions
    HTTP_ACCESS_NONE,                         // Edit permissions
    systemd_journal_directories_dyncfg_cb,    // Callback function
    NULL                                      // User data
);
```

Key points about its implementation:

- Uses a single, non-removable configuration object
- Supports schema, get, and update commands
- Validates directory paths for security
- Updates the systemd-journal watcher when configuration changes

### Go-based External Plugin (go.d.plugin)

Here's a complete example showing how a Go-based external plugin might implement DynCfg for an Nginx module:

### 1. Register the Template and Jobs on Startup

```
# Register the template for Nginx configurations
CONFIG go.d:nginx CREATE accepted template /collectors internal internal schema|add|enable|disable 0 0

# Register existing jobs
CONFIG go.d:nginx:local_server CREATE running job /collectors user /etc/netdata/go.d/nginx.conf schema|get|update|remove|enable|disable|restart 0 0
CONFIG go.d:nginx:production CREATE running job /collectors user /etc/netdata/go.d/nginx.conf schema|get|update|remove|enable|disable|restart 0 0
```

### 2. Handle Schema Command

When receiving:

```
FUNCTION abcd1234 60 "config go.d:nginx schema" "member" "netdata-cli"
```

Respond with:

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "format": "uri",
      "title": "Server URL",
      "description": "The URL of the Nginx stub_status endpoint"
    },
    "timeout": {
      "type": "integer",
      "minimum": 1,
      "maximum": 60,
      "title": "Timeout",
      "description": "Connection timeout in seconds"
    },
    "update_every": {
      "type": "integer",
      "minimum": 1,
      "title": "Update Every",
      "description": "Data collection frequency in seconds"
    }
  },
  "required": ["url"]
}
FUNCTION_RESULT_END
```

### 3. Handle Get Command

When receiving:

```
FUNCTION abcd1234 60 "config go.d:nginx:local_server get" "member" "netdata-cli"
```

Respond with:

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "url": "http://localhost:80/stub_status",
  "timeout": 5,
  "update_every": 10
}
FUNCTION_RESULT_END
```

### 4. Handle Update Command

When receiving:

```
FUNCTION_PAYLOAD_BEGIN abcd1234 60 "config go.d:nginx:local_server update" "member" "netdata-cli" "application/json"
{
  "url": "http://localhost:8080/stub_status",
  "timeout": 3,
  "update_every": 5
}
FUNCTION_PAYLOAD_END
```

Process the update and respond:

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "status": 200,
  "message": "Configuration updated successfully"
}
FUNCTION_RESULT_END
```

If a restart is required:

```
FUNCTION_RESULT_BEGIN abcd1234 299 application/json 0
{
  "status": 299,
  "message": "Configuration updated, restart required to apply changes"
}
FUNCTION_RESULT_END
```

### 5. Handle Add Command (for templates)

When receiving:

```
FUNCTION_PAYLOAD_BEGIN abcd1234 60 "config go.d:nginx add" "member" "netdata-cli" "application/json"
{
  "name": "staging",
  "url": "http://staging:80/stub_status",
  "timeout": 5,
  "update_every": 10
}
FUNCTION_PAYLOAD_END
```

Process the new job and respond:

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "status": 200,
  "message": "Job 'staging' created successfully"
}
FUNCTION_RESULT_END
```

Then register the new job:

```
CONFIG go.d:nginx:staging CREATE running job /collectors dyncfg netdata-cli schema|get|update|remove|enable|disable|restart 0 0
```

## Best Practices

1. **Use Consistent IDs**: Follow the pattern `component:template_name` for templates and `component:template_name:job_name` for jobs
2. **Validate Thoroughly**: Always validate configuration changes before accepting them
3. **Include Descriptive Messages**: Provide helpful error messages when rejections occur
4. **Document Your Schema**: Include clear titles and descriptions for all properties in your JSON Schema
5. **Handle Errors Gracefully**: Return appropriate HTTP status codes and error messages
6. **Update Status Promptly**: When a configuration changes state (e.g., from "accepted" to "running"), update its status
7. **Clean Up Configurations**: When a monitored resource is gone, delete its configuration with `CONFIG id DELETE`

## Debugging Tips

1. Set `NETDATA_DEBUG_DYNCFG=1` environment variable when running Netdata to see detailed logs
2. If configurations aren't being registered, check for errors in the plugin output
3. Verify configuration files are saved in `/var/lib/netdata/config/`
4. Test configurations via the API: `/api/v3/config?id=<your-config-id>`

## Related Documentation

- [Main DynCfg Documentation](/src/daemon/dyncfg/README.md) - Core DynCfg system concepts and APIs
- [Plugins.d Protocol](/src/plugins.d/README.md) - Complete documentation of the plugins.d protocol
