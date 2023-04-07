# Alerta Agent alert notifications

Learn how to send notifications to Alerta using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

The [Alerta](https://alerta.io) monitoring system is a tool used to consolidate and de-duplicate alerts from multiple sources for quick ‘at-a-glance’ visualization.
With just one system you can monitor alerts from many other monitoring tools on a single screen.

![Alerta dashboard showing several alerts](https://docs.alerta.io/_images/alerta-screen-shot-3.png)

Alerta's advantage is the main view, where you can see all active alert with the most recent state.
You can also view an alert history.

You can send Netdata alerts to Alerta to see alerts coming from many Netdata hosts or also from a multi-host Netdata configuration.

## Prerequisites

You need:

- an Alerta instance
- an Alerta API key (if authentication in Alerta is enabled)
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Alerta

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_ALERTA` to `YES`.
2. set `ALERTA_WEBHOOK_URL` to the API url you defined when you installed the Alerta server.
3. Set `ALERTA_API_KEY` to your API key.  
   You will need an API key to send messages from any source, if Alerta is configured to use authentication (recommended). To create a new API key:  
   1. Go to *Configuration* > *API Keys*.
   2. Create a new API key called "netdata" with `write:alerts` permission.
4. Set `DEFAULT_RECIPIENT_ALERTA` to the default recipient environment you want the alert notifications to be sent to.  
   All roles will default to this variable if left unconfigured.

You can then have different recipient environments per **role**, by editing `DEFAULT_RECIPIENT_CUSTOM` with the environment name you want, in the following entries at the bottom of the same file:

```conf
role_recipients_alerta[sysadmin]="Systems"
role_recipients_alerta[domainadmin]="Domains"
role_recipients_alerta[dba]="Databases Systems"
role_recipients_alerta[webmaster]="Marketing Development"
role_recipients_alerta[proxyadmin]="Proxy"
role_recipients_alerta[sitemgr]="Sites"
```

The values you provide should be defined as environments in `/etc/alertad.conf` option `ALLOWED_ENVIRONMENTS`.

An example working configuration would be:

```conf
#------------------------------------------------------------------------------
# alerta (alerta.io) global notification options

SEND_ALERTA="YES"
ALERTA_WEBHOOK_URL="http://yourserver/alerta/api"
ALERTA_API_KEY="INSERT_YOUR_API_KEY_HERE"
DEFAULT_RECIPIENT_ALERTA="Production"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
