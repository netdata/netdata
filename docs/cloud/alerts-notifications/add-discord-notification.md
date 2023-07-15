# Add Discord notification configuration

From the Netdata Cloud UI, you can manage your space's notification settings and enable the configuration to deliver notifications on Discord.

## Prerequisites

To enable Discord notifications you need:

- A Netdata Cloud account
- Access to the space as an **administrator**
- Have a Discord server able to receive webhook integrations. For more details check [how to configure this on Discord](#settings-on-discord)

## Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **Discord** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For Discord:
      - Define the type channel you want to send notifications to: **Text channel** or **Forum channel**
      - Webhook URL - URL provided on Discord for the channel you want to receive your notifications. For more details check [how to configure this on Discord](#settings-on-discord)
      - Thread name - if the Discord channel is a **Forum channel** you will need to provide the thread name as well

## Settings on Discord

## Enable webhook integrations on Discord server

To enable the webhook integrations on Discord you need:
1. Go to *Integrations** under your **Server Settings

   ![image](https://user-images.githubusercontent.com/82235632/214091719-89372894-d67f-4ec5-98d0-57c7d4256ebf.png)

1. **Create Webhook** or **View Webhooks** if you already have some defined
1. When you create a new webhook you specify: Name and Channel
1. Once you have this configured you will need the Webhook URL to add your notification configuration on Netdata UI

   ![image](https://user-images.githubusercontent.com/82235632/214092713-d16389e3-080f-4e1c-b150-c0fccbf4570e.png)

For more details please read this article from Discord: [Intro to Webhooks](https://support.discord.com/hc/en-us/articles/228383668).
