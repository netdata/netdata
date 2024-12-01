# Manage Alert Notification Silencing Rules

From the Cloud interface, you can manage your space's Alert notification silencing rules settings as well as allow users to define their personal ones.

## Prerequisites

To manage **space's Alert notification silencing rule settings**, you will need the following:

- A Netdata Cloud account
- Access to Space as an **administrator** or **manager** (**troubleshooters** can only view space rules)

To manage your **personal Alert notification silencing rule settings**, you will need the following:

- A Netdata Cloud account
- Access to Space with any role except **billing**

### Steps

1. Click on the **Space settings** cog (located above your profile icon).
2. Click on the **Alert & Notification** tab on the left-hand side.
3. Click on the **Notification Silencing Rules** tab.
4. You will be presented with a table of the configured Alert notification silencing rules for:

    - The space (if you aren't an **observer**)
    - Yourself

   You will be able to:

    1. **Add a new** Alert notification silencing rule configuration.
        - Choose if it applies to **All users** or **Myself** (All users is only available for **administrators** and **managers**).
        - You need to provide a name for the configuration so you can refer to it.
        - Define criteria for Nodes, to which Rooms will the rule apply, on what Nodes and whether it applies to host labels key-value pairs.
        - Define criteria for Alerts, such as Alert name is being targeted and in what Alert context. You can also specify if it applies to a specific Alert role.
        - Define when it is applied:
            - Immediately, from now until it is turned off or until a specific duration (start and end date automatically set).
            - Scheduled, you can specify the start and end time for when the rule becomes active and then inactive (time is set according to your browser's local timezone).
              Note: You are only able to add a rule if your space is on a [paid plan](/docs/netdata-cloud/view-plan-and-billing.md).
    2. **Edit an existing** Alert notification silencing rule configuration. You will be able to change:
        - The name provided for it
        - Who it applies to
        - Selection criteria for Nodes and Alerts
        - When it will be applied
    3. **Enable/Disable** a given Alert notification silencing rule configuration.
        - Use the toggle to enable or disable
    4. **Delete an existing** Alert notification silencing rule.
        - Use the trash icon to delete your configuration

## Silencing Rules Examples

| Rule name                        | Rooms              | Nodes    | Host Label               | Alert name                                       | Alert context | Alert instance           | Alert role  | Description                                                                                                                                                                                                               |
|:---------------------------------|:-------------------|:---------|:-------------------------|:-------------------------------------------------|:--------------|:-------------------------|:------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Space silencing                  | All Rooms          | *        | *                        | *                                                | *             | *                        | *           | This rule silences the entire space, targets all nodes, and for all users. E.g. infrastructure-wide maintenance window.                                                                                                   |
| DB Servers Rooms                 | PostgreSQL Servers | *        | *                        | *                                                | *             | *                        | *           | This rule silences the nodes in the Room named PostgreSQL Servers, for example, it doesn't silence the `All Nodes` Room. E.g. My team with membership to this Room doesn't want to receive notifications for these nodes. |
| Node child1                      | All Rooms          | `child1` | *                        | *                                                | *             | *                        | *           | This rule silences all Alert state transitions for node `child1` in all Rooms and for all users. E.g. node could be going under maintenance.                                                                              |
| Production nodes                 | All Rooms          | *        | `environment:production` | *                                                | *             | *                        | *           | This rule silences all Alert state transitions for nodes with the host label key-value pair `environment:production`. E.g. Maintenance window on nodes with specific host labels.                                         |
| Third party maintenance          | All Rooms          | *        | *                        | `httpcheck_posthog_netdata_cloud.request_status` | *             | *                        | *           | This rule silences this specific Alert since the third-party partner will be undergoing maintenance.                                                                                                                      |
| Intended stress usage on CPU     | All Rooms          | *        | *                        | *                                                | `system.cpu`  | *                        | *           | This rule silences specific Alerts across all nodes and their CPU cores.                                                                                                                                                  |
| Silence role webmaster           | All Rooms          | *        | *                        | *                                                | *             | *                        | `webmaster` | This rule silences all Alerts configured with the role `webmaster`.                                                                                                                                                       |
| Silence Alert on node            | All Rooms          | `child1` | *                        | `httpcheck_posthog_netdata_cloud.request_status` | *             | *                        | *           | This rule silences the specific Alert on the `child1` node.                                                                                                                                                               |
| Disk Space Alerts on mount point | All Rooms          | *        | *                        | `disk_space_usage`                               | `disk.space`  | `disk_space_opt_baddisk` | *           | This rule silences the specific Alert instance on all nodes `/opt/baddisk`.                                                                                                                                               |
