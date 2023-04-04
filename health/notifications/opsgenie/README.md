# Opsgenie Agent alert notifications

Learn how to send notifications to Opsgenie using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[Opsgenie](https://www.atlassian.com/software/opsgenie) is an alerting and incident response tool.
It is designed to group and filter alarms, build custom routing rules for on-call teams, and correlate deployments and commits to incidents.

This is what you will get:
![Example alarm notifications in
Opsgenie](https://user-images.githubusercontent.com/49162938/92184518-f725f900-ee40-11ea-9afa-e7c639c72206.png)

## Prerequisites

You will need:

- An Opsgenie integration. You can create an [integration](https://docs.opsgenie.com/docs/api-integration) in the [Opsgenie](https://www.atlassian.com/software/opsgenie) dashboard.

- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to your Opsgenie account

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_OPSGENIE` to `YES`.
2. Set `OPSGENIE_API_KEY` to the API key you got from Opsgenie.
3. `OPSGENIE_API_URL` defaults to `https://api.opsgenie.com`, however there are region-specific API URLs such as `https://eu.api.opsgenie.com`, so set this if required.

An example of a working configuration would be:

```conf
SEND_OPSGENIE="YES"
OPSGENIE_API_KEY="11111111-2222-3333-4444-555555555555"
OPSGENIE_API_URL=""
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
