# Manage Notification Methods

From the Cloud interface, you can manage your Space's notification settings as well as allow users to personalize their notification settings.

## Manage Space Notification Settings

### Prerequisites

To manage Space notification settings, you will need the following:

- A Netdata Cloud account
- Access to the Space as an **administrator**

### Available Actions per Notification Method Based on Service Level

| **Action**                                      | **Personal Service Level** | **System Service Level** |
|:------------------------------------------------|:--------------------------:|:------------------------:|
| Enable / Disable                                |             X              |            X             |
| Edit                                            |                            |            X             |
| Delete                                          |             X              |            X             |
| Add multiple configurations for the same method |                            |            X             |

> **Notes**
>
> - For Netdata provided ones, you can't delete the existing notification method configuration.
> - Enable, Edit, and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification)).

### Steps

1. Click on the **Space settings** cog (located above your profile icon).
2. Click on the **Alerts & Notifications** tab on the left-hand side.
3. Click on the **Notification Methods** tab.
4. You will be presented with a table of the configured notification methods for the Space. You will be able to:
   1. **Add a new** notification method configuration.
      - Choose the service from the list of available ones. The available options will depend on your subscription plan.
      - You can optionally provide a name for the configuration so you can easily refer to it.
      - You can define the filtering criteria, regarding which Rooms the method will apply, and what notifications you want to receive (All Alerts and unreachable, All Alerts, Critical only).
      - Depending on the service, different inputs will be present. Please note that there are mandatory and optional inputs.
         - If you have doubts on how to configure the service, you can find a link at the top of the modal that takes you to the specific documentation page to help you.
   2. **Edit an existing** notification method configuration. Personal level ones can't be edited here, see [Manage User Notification Settings](#manage-user-notification-settings). You will be able to change:
      - The name provided for it
      - Filtering criteria
      - Service-specific inputs
   3. **Enable/Disable** a given notification method configuration.
      - Use the toggle to enable or disable the notification method configuration.
   4. **Delete an existing** notification method configuration. Netdata provided ones can't be deleted, e.g., Email.
      - Use the trash icon to delete your configuration.

## Manage User Notification Settings

### Prerequisites

To manage user-specific notification settings, you will need the following:

- A Cloud account
- Access to, at least, a Space

Note: If an administrator has disabled a Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification method, this will override any user-specific setting.

### Steps

1. Click on the **User notification settings** shortcut on top of the help button.
2. You are presented with:
   - The Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification methods you can manage.
   - The list of Spaces and Rooms inside those where you have access to.
   - If you're an Administrator, Manager, or Troubleshooter, you'll also see the Rooms from a Space you don't have access to on the **All Rooms** tab, and you can activate notifications for them by joining the Room.
3. On this modal you will be able to:
   1. **Enable/Disable** the notification method for you; this applies across all Spaces and Rooms.
      - Use the toggle to enable or disable the notification method.
   2. **Define what notifications you want** per Space/room: All Alerts and unreachable, All Alerts, Critical only, or No notifications.
   3. **Activate notifications** for a Room you aren't a member of.
      - From the **All Rooms** tab, click on the Join button for the Room(s) you want.
