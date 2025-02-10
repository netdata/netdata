# Manage Notification Methods

From the Cloud interface, you can manage your Space's notification settings as well as allow users to personalize their notification settings.

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

> **Notes**
>
> - For Netdata provided ones, you can't delete the existing notification method configuration.
> - Enable, Edit, and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-classification)).

### Steps

1. Click on the **Space settings** cog (located above your profile icon).
2. Click on the **Alerts & Notifications** tab on the left-hand side.
3. Click on the **Notification Methods** tab.
4. You will be presented with a table of the configured notification methods for the Space. You will be able to:
    - **Add a new** notification method configuration.
        - Choose the service from the list of available ones. The available options will depend on your subscription plan.
        - You can optionally provide a name for the configuration so you can refer to it.
        - You can define the filtering criteria, regarding which Rooms the method will apply, and what notifications you want to receive (Critical, Warning, Clear, Reachable and Unreachable).
        - Depending on the service, different inputs will be present. Please note that there are mandatory and optional inputs.
            - If you have doubts on how to configure the service, you can find a link at the top of the modal that takes you to the specific documentation page to help you.
    - **Edit an existing** notification method configuration. Personal level ones can't be edited here, see [Manage User Notification Settings](#manage-user-notification-settings). You will be able to change:
        - The name provided for it
        - Filtering criteria
        - Service-specific inputs
    - **Enable/Disable** a given notification method configuration.
        - Use the toggle to enable or disable the notification method configuration.
    - **Delete an existing** notification method configuration. Netdata provided ones can't be deleted, e.g., Email.
        - Use the trash icon to delete your configuration.

## Manage User Notification Settings

### Prerequisites

To manage user-specific notification settings, you will need the following:

- A Cloud account
- Access to at least one Space

> **Note**
>
> If an administrator has disabled a Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification method, this will override any user-specific setting.

### Steps

1. Click on your profile picture and navigate to **Settings** -> **Notifications**
2. You are presented with:
    - The Personal [service level](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md#service-level) notification methods you can manage.
    - The list of Spaces and Rooms you have access to.
3. On this modal you will be able to:
    - **Enable/Disable** the notification method on a personal scope, this applies across all Spaces and Rooms.
    - **Define what notifications you want to receive** per Space/Room: Critical, Warning, Clear, Reachable and Unreachable.
    - **Join a Room and activate notifications**.
        - From the **All Rooms** tab, click on the Join button for the Room(s) you want.
