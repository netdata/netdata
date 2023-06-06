# Add Opsgenie notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on Opsgenie.

## Prerequisites

To add Opsgenie notification configurations you need:

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
      - Configuration name - you can optionally provide a name for your configuration you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For Opsgenie:
      - API Key - a key provided on Opsgenie for the channel you want to receive your notifications. For more details check [how to configure this on Opsgenie](#settings-on-opsgenie)

## Settings on Opsgenie

To enable the Netdata integration on Opsgenie you need:
1. Go to integrations tab of your team, click **Add integration**.

   ![image](https://user-images.githubusercontent.com/93676586/230361479-cb73919c-452d-47ec-8066-ed99be5f05e2.png)

1. Pick **API** from available integrations. Copy your API Key and press **Save Integration**.

1. Paste copied API key into the corresponding field in **Integration configuration** section of Opsgenie modal window in Netdata.
