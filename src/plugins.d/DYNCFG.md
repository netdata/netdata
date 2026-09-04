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

The Netdata UI (Cloud and the local dashboard) renders a job's configuration form from the document a plugin returns
for the `schema` command. That document is NOT a bare JSON Schema. It is an object with two members:

```json
{
  "jsonSchema": { "$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "properties": {} },
  "uiSchema": {}
}
```

- `jsonSchema`: a draft-07 JSON Schema describing the configuration object (types, defaults, `required`, `enum`,
  bounds). This drives the field types and the client-side validation.
- `uiSchema`: a react-jsonschema-form uiSchema, keyed by property name in parallel with `jsonSchema.properties`,
  carrying presentation hints (`ui:help`, `ui:placeholder`, `ui:widget`, tabs).

A response without the wrapper renders an EMPTY form: the UI reads `jsonSchema` and `uiSchema` and falls back to `{}`
for a missing member. The agent serves the document verbatim and validates nothing against it; the plugin's own
validation on `add`/`update` is the only server-side check.

### Static Schema Files (Optional)

Before calling the plugin, Netdata will first attempt to find a static schema file. You can provide static schema files in:

- `CONFIG_DIR/schema.d/` (user-provided schemas, typically `/etc/netdata/schema.d/`)
- `LIBCONFIG_DIR/schema.d/` (stock schemas, typically `/usr/lib/netdata/conf.d/schema.d/`)

Schema files should be named after the configuration ID with `.json` extension:

```
/etc/netdata/schema.d/go.d:nginx.json
```

This approach is useful for stable schemas that don't change frequently. Static files use the same two-member
wrapper.

### Dynamic Schema Generation

If no static schema file is found, Netdata will send a `schema` command to the plugin. The plugin returns the wrapper:

```json
{
  "jsonSchema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "Nginx collector configuration",
    "type": "object",
    "properties": {
      "update_every": {
        "title": "Update every",
        "description": "Data collection interval, in seconds.",
        "type": "integer",
        "minimum": 1,
        "default": 1
      },
      "url": {
        "title": "URL",
        "description": "URL of the Nginx stub_status page.",
        "type": "string",
        "format": "uri",
        "default": "http://127.0.0.1/stub_status"
      },
      "timeout": {
        "title": "Timeout",
        "description": "HTTP request timeout, in seconds.",
        "type": "number",
        "minimum": 0.5,
        "default": 1
      },
      "password": {
        "title": "Password",
        "description": "Password for HTTP basic authentication.",
        "type": "string"
      }
    },
    "required": ["url"]
  },
  "uiSchema": {
    "ui:flavour": "tabs",
    "ui:options": {
      "tabs": [
        { "title": "Base", "fields": ["update_every", "url", "timeout"] },
        { "title": "Auth", "fields": ["password"] }
      ]
    },
    "url": {
      "ui:help": "The `stub_status` page must be enabled in the Nginx configuration.\n\nExample:\n\n```\nlocation /stub_status { stub_status; }\n```"
    },
    "password": {
      "ui:widget": "password"
    }
  }
}
```

For templates, the schema will be used when users add new jobs based on the template.

### What The UI Renders

The form is react-jsonschema-form (v6) with Netdata templates and widgets. The table below is the vocabulary a plugin
schema uses. The UI also honors a few keys plugin forms do not need (`ui:groups` for grouped sections without tabs,
`ui:classNames` for grid layout, `ui:initiallyExpanded`, `ui:creatable` for free text in a select,
`ui:validation.warning`); any other `ui:*` key is silently ignored, except that a `ui:widget` value which is neither a
Netdata widget nor a react-jsonschema-form alias throws and replaces the form with an error view. This section
describes the UI as verified in September 2026; re-verify against the UI code when it changes.

Text channels, per property:

| Channel | Where it renders | Format |
|---|---|---|
| `title` | the field label; the section header for objects; falls back to the raw property name when absent | plain text |
| `description` | below the input (leaf), under the header (object), below all items (array) | Markdown |
| `ui:help` | an info icon next to the title; the text appears in a hover tooltip | Markdown |
| `ui:placeholder` | greyed text inside an empty string input | plain text |

Markdown facts that apply to `description` and `ui:help` alike:

- A single `\n` renders as a SPACE. Use `\n\n` for a new paragraph. Two trailing spaces or a trailing `\` force a
  line break.
- Supported: `#` headings, `**bold**`, `-` lists, `` `code` ``, fenced code blocks, `[links](url)` (open in a new
  tab), pipe tables (unstyled). Not supported: indented code blocks, bare-URL autolinks, raw HTML.
- The Markdoc tag syntax `{% ... %}` is active; a literal `{%` is interpreted, not printed.

`ui:*` keys the UI honors:

| Key | On | Effect |
|---|---|---|
| `ui:flavour: "tabs"` + `ui:options.tabs: [{title, fields}]` | object | renders the object's properties on tabs, tabs in array order; `fields` lists top-level property names of that object (no dotted paths); the order of fields inside a tab follows the object's property order (after `ui:order`), not the order in `fields`; any object at any depth may have its own tabs |
| `ui:options.rest: [names]` | tabbed object | properties rendered flat above the tab strip |
| `ui:help` | any property | info tooltip (Markdown) |
| `ui:placeholder` | string | input placeholder |
| `ui:widget` | leaf | `password` (masked input with reveal toggle), `textarea`, `radio`, `checkbox`, `select`, `hidden` (rendered nowhere, value kept) |
| `ui:options.inline: true` | radio | horizontal option layout |
| `ui:options.rows` | textarea | visible rows (default 2) |
| `ui:options.enumNames` | enum | display labels for `enum` values (the JSON Schema `enumNames` keyword is ignored) |
| `ui:listFlavour: "list"` | array | items stacked as a list instead of one-item-per-tab (tabs mode labels items `Rule N`) |
| `ui:collapsible: true` | object, array item | collapsible body |
| `ui:order: [names, "*"]` | object | property order |
| `ui:descriptionPosition: "top"` | any | description above the input |
| `ui:openEmptyItem: true` | array | adds one item when the array is empty |
| `ui:options.addable/orderable/removable: false` | array | hides the add, move, remove controls |
| `ui:options.collapsible`, `ui:options.flavour: "buttonGroup"`, `ui:options.enumOptions`, `ui:options.label: false` | object, radio, enum, any | collapsible alias; segmented radio; explicit option list; hides an object's title and description |
| `ui:title` | any | overrides `title` |

Behavior an author must design around:

- Tabs: a top-level property missing from every tab and from `ui:options.rest` is silently dropped from the UI, yet
  its `default` is still materialized and submitted. `ui:flavour: "tabs"` without `ui:options.tabs` renders an empty
  form. A property listed on two tabs renders twice, bound to one value. Two tabs with the same title collide.
- Defaults: every `default` in the schema is written into the form data on load, including inside sections the
  operator never opened, and is submitted with the job. An array with `minItems: N` is auto-filled with N items;
  declare `minItems` only on an array its parent requires or one with a `default`.
- Validation: the UI validates live with ajv (types, `required`, `enum`, `minimum`/`maximum`, `pattern`, `format`
  including `uri`, `ipv4`, `hostname`) and blocks the save while the form is invalid. A schema stricter than the
  plugin blocks legitimate configs; a looser one offers configs the plugin rejects.
- `dependencies` with `oneOf` on a `const` discriminator reveals the matching branch's properties inline (no selector
  widget); the UI drops the form data of inactive branches for TOP-LEVEL dependencies only. A key that is both a
  branch property and a plain sibling property renders unconditionally. Avoid property-level `oneOf`/`anyOf` (rendered
  as a branch selector whose first option cannot be selected reliably) and avoid `0`, `false`, and `""` as `enum`
  values in select-rendered fields.
- Nullable fields: a two-member union such as `["string", "null"]` renders as the non-null type; any other union
  (three or more members, or two members without `null`) collapses to its first member.
- Maps: `additionalProperties: {type: ...}` renders a key/value list with an add button; `patternProperties` alone
  renders without an add button.
- Secrets: `ui:widget: "password"` masks the input and is the ONLY signal that redacts the value in the live YAML
  preview pane. `format: "password"` masks the input but not the preview; any other schema flag (for example a custom
  `sensitive` field) is ignored by both.
- Unknown keys in an existing job's data are preserved and submitted; the schema is not a filter.

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

Respond with the two-member schema document described in "JSON Schema for Configuration UI":

```
FUNCTION_RESULT_BEGIN abcd1234 200 application/json 0
{
  "jsonSchema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "Nginx collector configuration",
    "type": "object",
    "properties": {
      "url": {
        "title": "URL",
        "description": "URL of the Nginx stub_status page.",
        "type": "string",
        "format": "uri"
      }
    },
    "required": ["url"]
  },
  "uiSchema": {
    "url": { "ui:placeholder": "http://127.0.0.1/stub_status" }
  }
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
4. **Document Your Schema**: Give every visible property a `title` and a plain-language `description`; put examples and
   longer explanations in `ui:help` (see "What The UI Renders")
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
