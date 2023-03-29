# Add Opsgenie notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on Opsgenie.

## Prerequisites

To add Opsgenie notification configurations you need

- A Netdata Cloud account
- Access to the space as an **administrator**
- Space on **Business** plan or higher
- Have a permission to add new integrations in Opsgenie.

## Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **Opsgenie** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For Slack:
      - Webhook URL - URL provided on Slack for the channel you want to receive your notifications. For more details check [how to configure this on Slack](#settings-on-slack)

## Settings on Opsgenie

To enable the Netdata integration on Opsgenie you need:
1. Go to integrations tab of your team, click **Add integration** and pick **Netdata** from available integrations. Copy API Key and press **Save Integration**.
1. Paste copied API key into the corresponding field in **Integration configuration** section of Opsgenie modal window in Netdata.
1. Configure Webhook URLs for your workspace
   - On your app go to **Incoming Webhooks** and click on **activate incoming webhooks**

   ![image](https://user-images.githubusercontent.com/2930882/214251948-486229bb-195b-499b-92e4-4be59a567a19.png)

   - At the bottom of **Webhook URLs for Your Workspace** section you have **Add New Webhook to Workspace**
   - After pressing that specify the channel where you want your notifications to be delivered

   ![image](https://user-images.githubusercontent.com/82235632/214103532-95f9928d-d4d6-4172-9c24-a4ddd330e96d.png)

   - Once completed copy the Webhook URL that you will need to add to your notification configuration on Netdata UI

   ![image](https://user-images.githubusercontent.com/82235632/214104412-13aaeced-1b40-4894-85f6-9db0eb35c584.png)

For more details please check Slacks's article [Incoming webhooks for Slack](https://slack.com/help/articles/115005265063-Incoming-webhooks-for-Slack).