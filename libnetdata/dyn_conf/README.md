# Netdata Dynamic Configuration

Purpose of Netdata Dynamic Configuration is to allow configuration of select Netdata plugins and options through the
Netdata API and by extension by UI.

## HTTP API documentation

### Summary API

For summary of all jobs and their statuses (for all children that stream to parent) use the following URL:

| Method  | Endpoint                      | Description                                                |
|:-------:|-------------------------------|------------------------------------------------------------|
| **GET** | `api/v2/job_statuses`         | list of Jobs                                               |
| **GET** | `api/v2/job_statuses?grouped` | list of Jobs (hierarchical, grouped by host/plugin/module) |

### Dyncfg API

### Top level

| Method  | Endpoint         | Description                             |
|:-------:|------------------|-----------------------------------------|
| **GET** | `/api/v2/config` | registered Plugins (sent DYNCFG_ENABLE) |

### Plugin level

| Method  | Endpoint                          | Description                  |
|:-------:|-----------------------------------|------------------------------|
| **GET** | `/api/v2/config/[plugin]`         | Plugin config                |
| **PUT** | `/api/v2/config/[plugin]`         | update Plugin config         |
| **GET** | `/api/v2/config/[plugin]/modules` | Modules registered by Plugin |
| **GET** | `/api/v2/config/[plugin]/schema`  | Plugin config schema         |

### Module level

| Method  | Endpoint                                      | Description               |
|:-------:|-----------------------------------------------|---------------------------|
| **GET** | `/api/v2/config/<plugin>/[module]`            | Module config             |
| **PUT** | `/api/v2/config/[plugin]/[module]`            | update Module config      |
| **GET** | `/api/v2/config/[plugin]/[module]/jobs`       | Jobs registered by Module |
| **GET** | `/api/v2/config/[plugin]/[module]/job_schema` | Job config schema         |
| **GET** | `/api/v2/config/[plugin]/[module]/schema`     | Module config schema      |

### Job level - only for modules where `module_type == job_array`

|   Method   | Endpoint                                 | Description                    |
|:----------:|------------------------------------------|--------------------------------|
|  **GET**   | `/api/v2/config/[plugin]/[module]/[job]` | Job config                     |
|  **PUT**   | `/api/v2/config/[plugin]/[module]/[job]` | update Job config              |
|  **POST**  | `/api/v2/config/[plugin]/[module]/[job]` | create Job                     |
| **DELETE** | `/api/v2/config/[plugin]/[module]/[job]` | delete Job (created by Dyncfg) |

## AGENT<->PLUGIN interface documentation

### 1. DYNCFG_ENABLE

Plugin signifies to agent its ability to use new dynamic config and the name it wishes to use by sending

```
plugin->agent:
=============
DYNCFG_ENABLE [plugin_url_name]
```

This can be sent only once per lifetime of the plugin (at startup or later) sending it multiple times is considered a
protocol violation and plugin might get terminated.
After this command is sent the plugin has to be ready to accept all the new commands/keywords related to dynamic
configuration (this command lets agent know this plugin is dyncfg capable and wishes to use dyncfg functionality).

After this command agent can call

```
agent->plugin:
=============
FUNCTION_PAYLOAD [UUID] 1 "set_plugin_config"
the new configuration
blah blah blah
FUNCTION_PAYLOAD_END

plugin->agent:
=============
FUNCTION_RESULT_BEGIN [UUID] [(1/0)(accept/reject)] [text/plain] 5
FUNCTION_RESULT_END
```

to set the new config which can be accepted/rejected by plugin by sending answer for this FUNCTION as it would with any
other regular function.

The new `FUNCTION_PAYLOAD` command differs from regular `FUNCTION` command exclusively in its ability to send bigger
payloads (configuration file contents) to the plugin (not just parameters list).

Agent can also call (after `DYNCFG_ENABLE`)

```
Agent->plugin:
=============
FUNCTION [UID] 1 "get_plugin_config"

Plugin->agent:
=============
FUNCTION_RESULT_BEGIN [UID] 1 text/plain 5
{
	"the currently used config from plugin" : "nice"
}
FUNCTION_RESULT_END
```

and

```
Agent->plugin:
=============
FUNCTION [UID] 1 "get_plugin_config_schema"

Plugin->agent:
=============
FUNCTION_RESULT_BEGIN [UID] 1 text/plain 5
{
	"the schema of plugin configuration" : "splendid"
}
FUNCTION_RESULT_END
```

Plugin can also register zero, one or more configurable modules using:

```
plugin->agent:
=============
DYNCFG_REGISTER_MODULE [module_url_name] (job_array|single)
```

modules can be added any time during plugins lifetime (you are not required to add them all at startup).

### 2. DYNCFG_REGISTER_MODULE

Module has to choose one of following types at registration:

- `single` - module itself has configuration but does not accept any jobs *(this is useful mainly for internal netdata
  configurable things like webserver etc.)*
- `job_array`  - module itself **can** *(not must)* have configuration and it has an array of jobs which can be added,
  modified and deleted. **this is what plugin developer needs in most cases**

After module has been registered agent can call

- `set_module_config [module]` FUNCTION_PAYLOAD
- `get_module_config [module]` FUNCTION
- `get_module_config_schema [module]` FUNCTION

with same syntax as `set_plugin_config` and `get_plugin_config`. In case of `set` command the plugin has ability to
reject the new configuration pushed to it.

In a case the module was registered as `job_array` type following commands can be used to manage jobs:

### 3. Job interface for job_array modules

- `get_job_config_schema [module]` - FUNCTION
- `get_job_config [module] [job]` - FUNCTION
- `set_job_config [module] [job]` - FUNCTION_PAYLOAD
- `delete_job_name [module] [job]` - FUNCTION

### 4. Streaming

When above commands are transferred trough streaming additionally `plugin_name` is prefixed as first parameter. This is
done to allow routing to appropriate plugin @child.

As a plugin developer you don't need to concern yourself with this detail as that parameter is stripped when sent to the
plugin *(and added when sent trough streaming)* automagically.
