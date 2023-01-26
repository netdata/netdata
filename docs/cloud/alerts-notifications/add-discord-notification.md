<!--
title: "Add discord notification configuration"
sidebar_label: "Add discord notification configuration"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/add-discord-notification-configuration.md"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations/Alerts"
learn_docs_purpose: "Instructions on how to add notification configuration for discord"
-->

From the Cloud interface, you can manage your space's notification settings and from these you can add specific configuration to get notifications delivered on discord.

#### Prerequisites

To add discord notification configurations you need

- A Cloud account
- Access to the space as and Administrator
- Have your discord server able to receive webhook integrations, for mode details check [how to configure this on discord](#settings-on-discord)

#### Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **discord** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For discord:
      - Define the type channel you want to send notifications to: **Text channel** or **Forum channel**
      - Webhook URL - URL provided on discord for the channel you want to receive your notifications. For more details check [how to configure this on discord](#settings-on-discord)
      - Thread name - if the discord channel is a **Forum channel** you will need to provide the thread name as well

#### Settings on discord

#### Enable webhook integrations on discord server

To enable the webhook integrations on discord you need:
1. Go to *Integrations** under your **Server Settings

   ![image](https://user-images.githubusercontent.com/82235632/214091719-89372894-d67f-4ec5-98d0-57c7d4256ebf.png)

1. **Create Webhook** or **View Webhooks** if you already have some defined
1. When you create a new webhook you specify: Name and Channel
1. Once you have this configured you will need the Webhook URL to add your notification configuration on Netdata UI

   ![image](https://user-images.githubusercontent.com/82235632/214092713-d16389e3-080f-4e1c-b150-c0fccbf4570e.png)

For more details please check discord's article [Intro to Webhooks](https://support.discord.com/hc/en-us/articles/228383668).


#### Related topics

- [Alerts](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md)
- [Alerts Configuration](https://github.com/netdata/netdata/blob/master/health/README.md)
- [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
- [Manage notification methods](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md)