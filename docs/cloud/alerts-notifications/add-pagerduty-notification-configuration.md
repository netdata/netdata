# Add PagerDuty notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on PagerDuty.

## Prerequisites

To add PagerDuty notification configurations you need:

- A Cloud account
- Access to the space as and **administrator**
- Space needs to be on **Business** plan or higher
- Have a PagerDuty service to receive events, for more details check [how to configure this on PagerDuty](#settings-on-pagerduty)

## Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **PagerDuty** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For PagerDuty:
      - Integration Key -  is a 32 character key provided by PagerDuty to receive events on your service. For more details check [how to configure this on PagerDuty](#settings-on-pagerduty)

## Settings on PagerDuty

## Enable webhook integrations on PagerDuty

To enable the webhook integrations on PagerDuty you need:
1. Create a service to receive events from your services directory page:

   ![image](https://user-images.githubusercontent.com/2930882/214254148-03714f31-7943-4444-9b63-7b83c9daa025.png)

1. At step 3, select `Events API V2` Integration:or **View Webhooks** if you already have some defined

   ![image](https://user-images.githubusercontent.com/2930882/214254466-423cf493-037d-47bd-b9e6-fc894897f333.png)

1. Once the service is created you will be redirected to its configuration page, where you can copy the **integration key**, that you will need need to add to your notification configuration on Netdata UI:

   ![image](https://user-images.githubusercontent.com/2930882/214255916-0d2e53d5-87cc-408a-9f5b-0308a3262d5c.png)
