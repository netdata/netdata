# Manage notification methods

From the Cloud interface, you can manage your space's notification settings as well as allow users to personalize their notifications setting

## Manage space notification settings

### Prerequisites

To manage space notification settings, you will need the following:

- A Netdata Cloud account
- Access to the space as an **administrator**

### Available actions per notification methods based on service level

| **Action** | **Personal service level** | **System service level** |
| :- | :-: | :-: |
| Enable / Disable | X | X |
| Edit | | X | |
| Delete | X | X |
| Add multiple configurations for same method | | X |

Notes:
* For Netadata provided ones you can't delete the existing notification method configuration.
* Enable, Edit and Add actions over specific notification methods will only be allowed if your plan has access to those ([service classification](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md#service-classification))

### Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Alerts & Notification** tab on the left hand-side
1. Click on the **Notification Methods** tab
1. You will be presented with a table of the configured notification methods for the space. You will be able to:
   1. **Add a new** notification method configuration.
      - Choose the service from the list of the available ones, you'll may see a list of unavailable options if your plan doesn't allow some of them (you will see on the
      card the plan level that allows a specific service)
      - You can optionally provide a name for the configuration so you can easily refer to what it
      - Define filtering criteria. To which Rooms will this apply? What notifications I want to receive? (All Alerts and unreachable, All Alerts, Critical only)
      - Depending on the service different inputs will be present, please note that there are mandatory and optional inputs
         - If you doubts on how to configure the service you can find a link at the top of the modal that takes you to the specific documentation page to help you
   1. **Edit an existing** notification method configuration. Personal level ones can't be edited here, see [Manage user notification settings](#manage-user-notification-settings). You will be able to change:
      - The name provided for it
      - Filtering criteria
      - Service specific inputs
   1. **Enable/Disable** a given notification method configuration.
      - Use the toggle to enable or disable the notification method configuration
   1. **Delete an existing** notification method configuration. Netdata provided ones can't be deleted, e.g. Email
      - Use the trash icon to delete your configuration 

## Manage user notification settings

### Prerequisites

To manage user specific notification settings, you will need the following:

- A Cloud account
- Have access to, at least, a space

Note: If an administrator has disabled a Personal [service level](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md#service-level) notification method this will override any user specific setting.

### Steps

1. Click on the **User notification settings** shortcut on top of the help button
1. You are presented with:
   - The Personal [service level](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md#service-level) notification methods you can manage
   - The list spaces and rooms inside those where you have access to
   - If you're an administrator, Manager or Troubleshooter you'll also see the Rooms from a space you don't have access to on **All Rooms** tab and you can activate notifications for them by joining the room
1. On this modal you will be able to:
   1. **Enable/Disable** the notification method for you, this applies accross all spaces and rooms
      - Use the toggle enable or disable the notification method
   1. **Define what notifications you want** to per space/room: All Alerts and unreachable, All Alerts, Critical only or No notifications
   1. **Activate notifications** for a room you aren't a member of
      - From the **All Rooms** tab click on the Join button for the room(s) you want

