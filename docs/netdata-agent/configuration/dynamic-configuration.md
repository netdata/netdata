# Dynamic Configuration Manager

You can edit the configuration of collectors and alerts through the Netdata UI.

Using this feature you can create, test and submit configurations for one or more nodes, without the need to get access on the node and use the terminal to create and load the configuration.

> **Note**
>
> - This feature is available on paid Spaces.
> - The node needs to be [connected to Cloud](/src/claim/README.md). This is done to ensure proper permission handling.
> - Viewing, Editing and Submitting jobs require the proper permissions
>   - Admins and Managers can edit and submit configuration jobs.

## Collectors

### Modules

Each module contains jobs. There are two actions available for modules:

- Add new job configurations
- Disable the module and all its configurations

### Jobs

Each job has a status and upon hovering an origin tag.

The tag explains where the configuration job originates from:

- **Stock**: Configuration comes by default upon installing Netdata.
- **User**: Configuration comes from a user-made file.
- **Discovered**: Configuration comes from a service that was auto-discovered on the node.
- **Dynamic Configuration**: Configuration was made using the Dynamic Configuration Manager.

> **Note**
>
> Only Dynamic Configuration jobs can be deleted, as other types do not originate from the UI and are actual files on the node.

From this view you can also take the following actions for configurations:

- **Restart**: This helps in the occasion where a job has the **Failed** status. Upon restarting, a notification with the failure message will be displayed.
- **Remove**: You can delete the job configuration.
- **Disable**: You can disable this specific job configuration.
- **Edit configuration**: See [the following section](#editing-job-configurations) to see what this view provides.

> **Tip**
>
> You can also use the filter on the right-hand side to narrow down the list, for example showing only failed jobs or ones that need restarting.

#### Editing job configurations

From the configuration editing panel you can build your configuration and submit it to one or more nodes.

In detail you can:

- See the job's status.
- Edit the various configuration sections.
  - Any edits you make will be reflected in the right-hand panel's configuration string.
- See the formatted configuration string and copy it
- Test the configuration
  - This action tests your configuration to see if it would be able to find any metrics on the given node.
- Submit the configuration to one or any available nodes
  - By submitting the configuration, the Agent either accepts it and enables it or rejects it if it finds any errors.

## Health

Each entry in the Health tab contains an Alert template, that then is used to create Alerts.

The functionality in the main view is the same as with the [Collectors tab](#collectors).

### Health entry configuration

You can create new configurations both for templates or individual Alerts.

Each template can have multiple items which resemble Alerts that either apply to a certain [instance](/docs/dashboards-and-charts/netdata-charts.md#instances-dropdown), or all instances under a specific [context](/docs/dashboards-and-charts/netdata-charts.md#contexts)
