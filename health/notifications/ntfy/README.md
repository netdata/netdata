# ntfy agent alert notifications

Learn how to send alerts to an ntfy server using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[ntfy](https://ntfy.sh/) (pronounce: notify) is a simple HTTP-based [pub-sub](https://en.wikipedia.org/wiki/Publish%E2%80%93subscribe_pattern) notification service. It allows you to send notifications to your phone or desktop via scripts from any computer, entirely without signup, cost or setup. It's also [open source](https://github.com/binwiederhier/ntfy) if you want to run your own server. 

This is what you will get:

<img src="https://user-images.githubusercontent.com/5953192/230661442-a180abe2-c8bd-496e-88be-9038e62fb4f7.png" alt="Example alarm notifications in Ntfy" width="60%"></img>

## Prerequisites

You will need:

- (Optional) A [self-hosted ntfy server](https://docs.ntfy.sh/faq/#can-i-self-host-it), in case you don't want to use https://ntfy.sh
- A new [topic](https://ntfy.sh/#subscribe) for the notifications to be published to
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to ntfy

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_NTFY` to `YES`
2. Set `DEFAULT_RECIPIENT_NTFY` to the URL formed by the server-topic combination you want the alert notifications to be sent to. Unless you are hosting your own server, the server should always be set to [https://ntfy.sh](https://ntfy.sh)

    You can define multiple recipient URLs like this: `https://SERVER1/TOPIC1 https://SERVER2/TOPIC2`  
    All roles will default to this variable if left unconfigured.

> ### Warning
> All topics published on https://ntfy.sh are public, so anyone can subscribe to them and follow your notifications. To avoid that, ensure the topic is unique enough using a long, randomly generated ID, like in the following examples.
> 

An example of a working configuration with two topics as recipients, using the [https://ntfy.sh](https://ntfy.sh) server would be:

```conf
SEND_NTFY="YES"
DEFAULT_RECIPIENT_NTFY="https://ntfy.sh/netdata-X7seHg7d3Tw9zGOk https://ntfy.sh/netdata-oIPm4IK1IlUtlA30" 
```

You can then have different servers and/or topics per **role**, by editing `DEFAULT_RECIPIENT_NTFY` with the server-topic combination you want, in the following entries at the bottom of the same file:

```conf
role_recipients_ntfy[sysadmin]="https://SERVER1/TOPIC1"
role_recipients_ntfy[domainadmin]="https://SERVER2/TOPIC2"
role_recipients_ntfy[dba]="https://SERVER3/TOPIC3"
role_recipients_ntfy[webmaster]="https://SERVER4/TOPIC4"
role_recipients_ntfy[proxyadmin]="https://SERVER5/TOPIC5"
role_recipients_ntfy[sitemgr]="https://SERVER6/TOPIC6"
```

## Test the notification method

To test this alert refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
