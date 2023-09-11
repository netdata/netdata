# Add Mattermost notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on Mattermost.

## Prerequisites

To add Mattermost notification configurations you need:

- A Netdata Cloud account
- Access to the space as an **administrator**
- Space needs to be on **Business** plan or higher
- Have a Mattermost app on your workspace to receive the webhooks, for more details check [how to configure this on Mattermost](#settings-on-mattermost)

## Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **Mattermost** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For Mattermost:
      - Webhook URL - URL provided on Mattermost for the channel you want to receive your notifications. For more details check [how to configure this on Mattermost](#settings-on-mattermost)

## Settings on Mattermost

To enable the webhook integrations on Mattermost you need:
1. In Mattermost, go to Product menu > Integrations > Incoming Webhook.

![image](https://user-images.githubusercontent.com/26550862/243394526-6d45f6c2-c3cc-4d5f-a9cb-85d8170fc8ac.png)

   - If you don’t have the Integrations option, incoming webhooks may not be enabled on your Mattermost server or may be disabled for non-admins. They can be enabled by a System Admin from System Console > Integrations > Integration Management. Once incoming webhooks are enabled, continue with the steps below

![image](https://user-images.githubusercontent.com/26550862/243394734-f911ccf7-bb18-41b2-ab52-31195861dd1b.png)

2. Select Add Incoming Webhook and add a name and description for the webhook. The description can be up to 500 characters

3. Select the channel to receive webhook payloads, then select Add to create the webhook

![image](https://user-images.githubusercontent.com/26550862/243394626-363b7cbc-3550-47ef-b2f3-ce929919145f.png)

4. You will end up with a webhook endpoint that looks like so:
```
https://your-mattermost-server.com/hooks/xxx-generatedkey-xxx
```
   - Treat this endpoint as a secret. Anyone who has it will be able to post messages to your Mattermost instance.

For more details please check Mattermost's article [Incoming webhooks for Mattermost](https://developers.mattermost.com/integrate/webhooks/incoming/).
