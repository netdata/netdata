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

## Internal Plugins API

TBD

## External Plugins API

### Commands plugins can use

#### DYNCFG_ENABLE

Plugin signifies to agent its ability to use new dynamic config and the name it wishes to use by sending

```  
DYNCFG_ENABLE [{PLUGIN_NAME}]
```  

This can be sent only once per lifetime of the plugin (at startup or later) sending it multiple times is considered a  
protocol violation and plugin might get terminated.

After this command is sent the plugin has to be ready to accept all the new commands/keywords related to dynamic  
configuration (this command lets agent know this plugin is dyncfg capable and wishes to use dyncfg functionality).

#### DYNCFG_RESET

Sending this, will reset the internal state of the agent, considering this a `DYNCFG_ENABLE`.

```  
DYNCFG_RESET
```  


#### DYNCFG_REGISTER_MODULE

```
DYNCFG_REGISTER_MODULE {MODULE_NAME} {MODULE_TYPE}
```

Module has to choose one of following types at registration:

- `single` - module itself has configuration but does not accept any jobs *(this is useful mainly for internal netdata  
  configurable things like webserver etc.)*

- `job_array` - module itself **can** *(not must)* have configuration and it has an array of jobs which can be added,  
  modified and deleted. **this is what plugin developer needs in most cases**

After a module has been registered agent can call `set_module_config`, `get_module_config` and `get_module_config_schema`.

When `MODULE_TYPE` is `job_array` the agent may also send  `set_job_config`, `get_job_config` and `get_job_config_schema`.

#### DYNCFG_REGISTER_JOB

The plugin can use `DYNCFG_REGISTER_JOB` to register its own configuration jobs. It should not register jobs configured
via DYNCFG (doing so, the agent will shutdown the plugin).


```
DYNCFG_REGISTER_JOB {MODULE_NAME} {JOB_NAME} {JOB_TYPE} {FLAGS}
```

Where:

- `MODULE_NAME` is the name of the module.
- `JOB_NAME` is the name of the job.
- `JOB_TYPE` is either `stock` or `autodiscovered`.
- `FLAGS`, just send zero.

#### REPORT_JOB_STATUS

```
REPORT_JOB_STATUS {MODULE_NAME} {JOB_NAME} {STATUS} {STATE} "{REASON}"
```

Where:

- `MODULE_NAME` is the name of the module.
- `JOB_NAME` is the name of the job.
- `STATUS` is one of `stopped`, `running`, or `error`.
- `STATE`, just send zero.
- `REASON` is a message describing the status.


### Commands plugins must serve

Once a plugin calls `DYNCFG_ENABLE`, the must be able to handle these calls.

function|parameters|prerequisites|request payload|response payload|
:---:|:---:|:---:|:---:|:---:|
`set_plugin_config`|none|`DYNCFG_ENABLE`|plugin configuration|none|
`get_plugin_config`|none|`DYNCFG_ENABLE`|none|plugin configuration|
`get_plugin_config_schema`|none|`DYNCFG_ENABLE`|none|plugin configuration schema|
`set_module_config`|`module_name`|`DYNCFG_REGISTER_MODULE`|module configuration|none|
`get_module_config`|`module_name`|`DYNCFG_REGISTER_MODULE`|none|module configuration|
`get_module_config_schema`|`module_name`|`DYNCFG_REGISTER_MODULE`|none|module configuration schema|
`set_job_config`|`module_name`, `job_name`|`DYNCFG_REGISTER_MODULE`|job configuration|none|
`get_job_config`|`module_name`, `job_name`|`DYNCFG_REGISTER_MODULE`|none|job configuration|
`get_job_config_schema`|`module_name`, `job_name`|`DYNCFG_REGISTER_MODULE`|none|job configuration schema|

All of them work like this:

If the request payload is `none`, then the request looks like this:

```bash
FUNCTION {TRANSACTION_UUID} {TIMEOUT_SECONDS} "{function} {parameters}"  
```

When there is payload, the request looks like this:

```bash
FUNCTION_PAYLOAD {TRANSACTION_UUID} {TIMEOUT_SECONDS} "{function} {parameters}" 
<payload>
FUNCTION_PAYLOAD_END  
```

In all cases, the response is like this:

```bash
FUNCTION_RESULT_BEGIN {TRANSACTION_UUID} {HTTP_RESPONSE_CODE} "{CONTENT_TYPE}" {EXPIRATION_TIMESTAMP}
<payload>
FUNCTION_RESULT_END  
```
Where:
- `TRANSACTION_UUID` is the same UUID received with the request.
- `HTTP_RESPONSE_CODE` is either `0` (rejected) or `1` (accepted).
- `CONTENT_TYPE` should reflect the `payload` returned.
- `EXPIRATION_TIMESTAMP` can be zero.


## DYNCFG with streaming

When above commands are transferred trough streaming additionally `plugin_name` is prefixed as first parameter. This is  
done to allow routing to appropriate plugin @child.

As a plugin developer you don't need to concern yourself with this detail as that parameter is stripped when sent to the  
plugin *(and added when sent trough streaming)* automagically.
