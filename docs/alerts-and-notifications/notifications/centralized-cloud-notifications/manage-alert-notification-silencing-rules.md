# Manage alert notification silencing rules

From the Cloud interface, you can manage your space's alert notification silencing rules settings as well as allow users to define their personal ones.

## Prerequisites

To manage **space's alert notification silencing rule settings**, you will need the following:

- A Netdata Cloud account
- Access to the space as an **administrator** or **manager** (**troubleshooters** can only view space rules)


To manage your **personal alert notification silencing rule settings**, you will need the following:

- A Netdata Cloud account
- Access to the space with any roles except **billing**

### Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Alert & Notification** tab on the left hand-side
1. Click on the **Notification Silencing Rules** tab
1. You will be presented with a table of the configured alert notification silencing rules for:
  * the space (if aren't an **observer**)
  * yourself
  
  You will be able to:
   1. **Add a new** alert notification silencing rule configuration.
      - Choose if it applies to **All users** or **Myself** (All users is only available for **administrators** and **managers**)
      - You need to provide a name for the configuration so you can easily refer to it
      - Define criteria for Nodes: To which Rooms will this apply? What Nodes? Does it apply to host labels key-value pairs?
      - Define criteria for Alerts: Which alert name is being targeted? What alert context? Will it apply to a specific alert role?
      - Define when it will be applied:
        - Immediately, from now till until it is turned off or until a specific duration (start and end date automatically set)
        - Scheduled, you specify the start and end time for when the rule becomes active and then inactive (time is set according to your browser local timezone)
      Note: You are only able to add a rule if your space is on a [paid plan](https://github.com/netdata/netdata/edit/master/docs/cloud/manage/plans.md).
   1. **Edit an existing** alert notification silencing rule configurations. You will be able to change:
      - The name provided for it
      - Who it applies to
      - Selection criteria for Nodes and Alert
      - When it will be applied
   1. **Enable/Disable** a given alert notification silencing rule configuration.
      - Use the toggle to enable or disable
   1. **Delete an existing** alert notification silencing rule.
      - Use the trash icon to delete your configuration 

## Silencing rules examples

| Rule name | War Rooms | Nodes | Host Label | Alert name | Alert context | Alert instance | Alert role | Description |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- | :-- | :--|
| Space silencing | All Rooms | * | * | * | * | * | This rule silences the entire space, targets all nodes and for all users. E.g. infrastructure wide maintenance window. |
| DB Servers Rooms | PostgreSQL Servers | * | * | * | * | * | * | This rules silences the nodes in the room named PostgreSQL Servers, for example it doesn't silence the `All Nodes` room. E.g. My team with membership to this room doesn't want to receive notifications for these nodes. |
| Node child1 | All Rooms | `child1` | * | * | * | * | * | This rule silences all alert state transitions for node `child1` on all rooms and for all users. E.g. node could be going under maintenance. |
| Production nodes | All Rooms | * | `environment:production` | * | * | * | * | This rule silences all alert state transitions for nodes with the host label key-value pair `environment:production`. E.g. Maintenance window on nodes with specific host labels. |
| Third party maintenance | All Rooms | * | * | `httpcheck_posthog_netdata_cloud.request_status` | * | * | * | This rule silences this specific alert since third party partner will be undergoing maintenance. |
| Intended stress usage on CPU | All Rooms | * | * | * | `system.cpu` | * | * | This rule silences specific alerts across all nodes and their CPU cores. |
| Silence role webmaster | All Rooms | * | * | * | * | * | `webmaster` | This rule silences all alerts configured with the role `webmaster`. |
| Silence alert on node | All Rooms | `child1` | * | `httpcheck_posthog_netdata_cloud.request_status` | * | * | * | This rule silences the specific alert on the `child1` node. |
| Disk Space alerts on mount point | All Rooms | * | * | `disk_space_usage` | `disk.space` | `disk_space_opt_baddisk` | * | This rule silences the specific alert instance on all nodes `/opt/baddisk`. |
