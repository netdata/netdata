# Gotify agent alert notifications

Learn how to send alerts to your Gotify instance using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[Gotify](https://gotify.net/) is a self-hosted push notification service created for sending and receiving messages in real time.

This is what you will get:

<img src="https://user-images.githubusercontent.com/103264516/162509205-1e88e5d9-96b6-4f7f-9426-182776158128.png" alt="Example alarm notifications in Gotify" width="70%"></img>

## Prerequisites

You will need:

- An application token. You can generate a new token in the Gotify Web UI.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Gotify

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_GOTIFY` to `YES`
2. Set `GOTIFY_APP_TOKEN` to the app token you generated
3. `GOTIFY_APP_URL` to point to your Gotify instance, for example `https://push.example.domain/`

An example of a working configuration would be:

```conf
SEND_GOTIFY="YES"
GOTIFY_APP_TOKEN="XXXXXXXXXXXXXXX"
GOTIFY_APP_URL="https://push.example.domain/"
```

## Test the notification method

To test this alert refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
