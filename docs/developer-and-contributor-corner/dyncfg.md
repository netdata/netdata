# Dynamic Configuration (DynCfg)

Dynamic Configuration (DynCfg) is a system in Netdata that enables both internal and external plugins/modules to expose their configurations dynamically to users through a unified interface. This document provides an overview of the DynCfg system and directs developers to detailed implementation documentation.

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
- Configuration changes are validated by the owning plugin before being committed
- The DynCfg manager maintains the state of all dynamic configurations
- JSON Schema is used to define the structure of configuration objects
- The UI is based on adaptations of the react-jsonschema-form project

## Architecture

DynCfg consists of these key components:

1. **DynCfg Manager**: Core system that tracks configurations and routes commands
2. **Internal Plugin API**: Used by modules inside the Netdata agent
3. **External Plugin API**: Used by independent plugins communicating via plugins.d protocol
4. **Web API**: Exposes configuration management to users and applications

## Configuration Types

DynCfg supports three types of configurations:

- **SINGLE**: A standalone configuration object (e.g., systemd-journal directories)
- **TEMPLATE**: A blueprint for creating multiple related configurations (e.g., Nginx collector template)
- **JOB**: A specific configuration instance derived from a template (e.g., a specific Nginx server to monitor)

## Implementation Documentation

For detailed implementation guidance, refer to these documents:

### For Internal Modules/Plugins

If you're developing an internal Netdata module or plugin, see:

👉 [**Internal DynCfg Implementation Guide**](/src/daemon/dyncfg/README.md)

This document covers:
- Low-level and high-level APIs
- Configuration ID structure
- Response codes and status handling
- Action behavior for different configuration types
- JSON Schema implementation
- API access and endpoints
- Best practices

### For External Plugins

If you're developing an external plugin that communicates with Netdata using the plugins.d protocol, see:

👉 [**External Plugin DynCfg Implementation Guide**](/src/plugins.d/DYNCFG.md)

This document covers:
- Plugin protocol commands and responses
- Registering configurations
- Handling configuration commands
- Responding to status changes
- Schema handling
- Working examples

## Example Implementations

For reference, you can study these existing implementations:

### Health Alerts System (Internal)

The health module uses DynCfg to manage alert definitions. Key files:
- `src/health/health_dyncfg.c`: Implements DynCfg integration for health alerts
- Uses the high-level API for internal plugins

### systemd-journal.plugin (External)

The systemd-journal.plugin is a C-based external plugin that uses DynCfg. Key files:
- `src/collectors/systemd-journal.plugin/systemd-journal-dyncfg.c`: Implements a SINGLE configuration for journal directories

### go.d.plugin (External)

go.d.plugin is a Go-based external plugin that uses DynCfg to manage job configurations:
- Implements templates and jobs for various data collectors
- Dynamically generates JSON Schema based on Go struct tags

## Best Practices

When implementing DynCfg for your module or plugin:

1. **Use Clear ID Structure**: Follow the component:category:name pattern
2. **Choose Logical Paths**: The path parameter affects UI organization
3. **Validate Thoroughly**: Always validate configuration changes before accepting
4. **Provide Detailed Errors**: Help users understand why a configuration was rejected
5. **Document Your Schema**: Include good descriptions in your JSON Schema
6. **Respect Type-Action Relationships**: Different actions behave differently for each configuration type
7. **Return Appropriate Status Codes**: Use the correct response codes for each situation

## More Information

For more details about using the Netdata Agent UI to manage dynamic configurations, see the [Netdata Cloud documentation](https://learn.netdata.cloud/docs/agent/web/gui/).

To learn about developing for Netdata, see the [Developer Corner](https://learn.netdata.cloud/docs/agent/contribute/).