# Slack Agent alert notifications

Learn how to send notifications to a Slack workspace using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:

![image](https://user-images.githubusercontent.com/70198089/229841857-77ed2562-ee62-427b-803a-cef03d08238d.png)


## Prerequisites

You will need:

- a Slack app along with an incoming webhook, read Slack's guide on the topic [here](https://api.slack.com/messaging/webhooks)
- one or more channels to post the messages to
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Slack

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_SLACK` to `YES`.
2. Set `SLACK_WEBHOOK_URL` to your Slack app's webhook URL.
3. Set `DEFAULT_RECIPIENT_SLACK` to the Slack channel your Slack app is set to send messages to.  
   The syntax for channels is `#channel` or `channel`.  
   All roles will default to this variable if left unconfigured.

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# slack (slack.com) global notification options

SEND_SLACK="YES"
SLACK_WEBHOOK_URL="https://hooks.slack.com/services/XXXXXXXX/XXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX" 
DEFAULT_RECIPIENT_SLACK="#alarms"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
