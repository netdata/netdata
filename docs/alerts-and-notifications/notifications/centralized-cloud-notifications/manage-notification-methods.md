# Manage Notification Methods

From the Cloud interface, you can manage your Space's notification settings as well as allow users to personalize their notification settings.

## Notification Types Reference

Netdata supports five types of notifications that you can configure:

| **Notification Type** | **Description**                                               |
|:----------------------|:--------------------------------------------------------------|
| **Critical**          | Alerts indicating serious problems requiring immediate action |
| **Warning**           | Alerts showing concerning behavior that requires attention    |
| **Clear**             | Notifications when alerts return to normal state              |
| **Reachable**         | Notifications when nodes come back online                     |
| **Unreachable**       | Notifications when nodes go offline or become unreachable     |

## Manage Space Notification Settings

### Prerequisites

To manage Space notification settings, you will need the following:

- A Netdata Cloud account
- Access to Space as an **administrator**

### Available Actions per Notification Method Based on Service Level

| **Action**                                      | **Personal Service Level** | **System Service Level** |
|:------------------------------------------------|:--------------------------:|:------------------------:|
| Enable / Disable                                |             X              |            X             |
| Edit                                            |                            |            X             |
| Delete                                          |             X              |            X             |
| Add multiple configurations for the same method |                            |            X             |

:::note

- For Netdata provided ones, you can't delete the existing notification method configuration
- Enable, Edit, and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification))

:::

### Steps to Configure Space Notifications

1. Click on the **Space settings** cog (located above your profile icon).
2. Click on the **Alerts & Notifications** tab on the left-hand side.
3. Click on the **Notification Methods** tab.

<details>
<summary>4. <strong>Manage Notification Methods</strong></summary><br/>

You will be presented with a table of the configured notification methods for the Space. You will be able to:

- **Add a new** notification method configuration:
    - Choose the service from the list of available ones. The available options will depend on your subscription plan.

  :::tip

    - You can optionally provide a name for the configuration so you can refer to it.
    - You can define the filtering criteria, regarding which Rooms the method will apply, and what notifications you want to receive (Critical, Warning, Clear, Reachable and Unreachable).

  :::

    - Depending on the service, different inputs will be present.

  :::note

  Please note that there are mandatory and optional inputs.

  :::

  :::tip

    - If you have doubts on how to configure the service, you can find a link at the top of the modal that takes you to the specific documentation page to help you.

  :::

- **Edit an existing** notification method configuration. Personal level ones can't be edited here, see [Manage User Notification Settings](#manage-user-notification-settings). You will be able to change:
    - The name provided for it
    - Filtering criteria
    - Service-specific inputs

- **Enable/Disable** a given notification method configuration:
    - Use the toggle to enable or disable the notification method configuration

- **Delete an existing** notification method configuration. Netdata provided ones can't be deleted, e.g., Email:
    - Use the trash icon to delete your configuration

</details>

## Testing Your Notification Setup

You can verify that your Cloud notification setup is working correctly by using the Agent's test functionality on any connected node:

<details>
<summary><strong>Basic Test Commands</strong></summary><br/>

```bash
# Switch to the Netdata user
sudo su -s /bin/bash netdata

# Enable debugging
export NETDATA_ALARM_NOTIFY_DEBUG=1

# Test default role (sysadmin)
./plugins.d/alarm-notify.sh test

# Test specific role
./plugins.d/alarm-notify.sh test "webmaster"
```

</details>

## What Happens After Configuration

When you complete your notification setup:

1. **Configuration takes effect immediately** - Changes are applied as soon as you save them
2. **All matching alerts will trigger notifications** - Based on your filtering criteria
3. **You can monitor notification delivery** - Check your configured channels for test messages
4. **Flood protection activates automatically** - To prevent notification overload from frequently changing nodes

## Manage User Notification Settings

### Prerequisites

To manage user-specific notification settings, you will need the following:

- A Cloud account
- Access to at least one Space

:::note

If an administrator has disabled a Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification method, this will override any user-specific setting.

:::

### Steps to Configure Personal Notifications

<details>
<summary><strong>Access User Settings</strong></summary><br/>

Click on your profile picture and navigate to **Settings** → **Notifications**
</details>

<details>
<summary><strong>Review Available Options</strong></summary><br/>

You are presented with:

- The Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification methods you can manage
- The list of Spaces and Rooms you have access to

</details>

<details>
<summary><strong>Configure Your Notifications</strong></summary><br/>

On this modal you will be able to:

- **Enable/Disable** the notification method on a personal scope, this applies across all Spaces and Rooms
- **Define what notifications you want to receive** per Space/Room: Critical, Warning, Clear, Reachable and Unreachable
- **Join a Room and activate notifications**:
    - From the **All Rooms** tab, click on the Join button for the Room(s) you want

</details>

## Best Practices

Based on Netdata's documentation, here are recommended practices:

### Filtering Strategy

- Use **Room-based filtering** to organize notifications by infrastructure components
- Apply **severity filtering** to route Critical alerts to immediate channels (like PagerDuty) and Warning alerts to monitoring channels (like Slack)
- Utilize **host labels** for granular targeting of specific infrastructure segments

### Avoiding Alert Fatigue

- Netdata includes built-in **flood protection** that limits notifications when nodes frequently change state

:::tip

- Configure different notification methods for different severity levels
- Use **silencing rules** during planned maintenance windows

:::

## Related Documentation

- [Alert notification silencing rules](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-alert-notification-silencing-rules.md)
- [Service classification reference](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification)
- [Agent notification testing](/src/health/notifications/README.md#testing-your-notification-setup)
