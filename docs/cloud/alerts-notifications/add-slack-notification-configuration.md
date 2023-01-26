<!--
title: "Add slack notification configuration"
sidebar_label: "Add discord notification configuration"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/add-slack-notification-configuration.md"
sidebar_position: "1"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations/Alert Notifications"
learn_docs_purpose: "Instructions on how to add notification configuration for slack"
-->

From the Cloud interface, you can manage your space's notification settings and from these you can add specific configuration to get notifications delivered on slack.

#### Prerequisites

To add discord notification configurations you need

- A Cloud account
- Access to the space as and **Administrator**
- Space will needs to be on **Business** plan or higher
- Have a slack app on your workspace to receive the webhooks, for mode details check [how to configure this on slack](#settings-on-slack)

#### Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **slack** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For slack:
      - Webhook URL - URL provided on slack for the channel you want to receive your notifications. For more details check [how to configure this on slack](#settings-on-slack)

#### Settings on slack

To enable the webhook integrations on slack you need:
1. Create an app to receive webhook integrations. Check [Create an app](https://api.slack.com/apps?new_app=1)
1. Install the app on your workspace
1. Configure Webhook URLs for your workspace
   - On your app go to **Incoming Webhooks** and click on **activate incoming webhooks**

   ![image](https://user-images.githubusercontent.com/2930882/214251948-486229bb-195b-499b-92e4-4be59a567a19.png)
   
   - At the bottom of **Webhook URLs for Your Workspace** section you have **Add New Webhook to Workspace**
   - After pressing that specify the channel where you want your notifications to be delivered

   ![image](https://user-images.githubusercontent.com/82235632/214103532-95f9928d-d4d6-4172-9c24-a4ddd330e96d.png)

   - Once completed copy the Webhook URL that you will need to add to your notification configuration on Netdata UI

   ![image](https://user-images.githubusercontent.com/82235632/214104412-13aaeced-1b40-4894-85f6-9db0eb35c584.png)

For more details please check slacks's article [Incoming webhooks for Slack](https://slack.com/help/articles/115005265063-Incoming-webhooks-for-Slack).


#### Related topics

- [Alerts](https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/alerts.md)
- [Alerts Configuration](https://github.com/netdata/netdata/blob/master/health/README.md)
- [Centralized Alerts](https://github.com/netdata/netdata/blob/master/docs/concepts/netdata-cloud/centralized-alerts.md)
- [Manage notification methods](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md)