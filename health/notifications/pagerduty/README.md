# PagerDuty Agent alert notifications

Learn how to send notifications to PagerDuty using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[PagerDuty](https://www.pagerduty.com/company/) is an enterprise incident resolution service that integrates with ITOps
and DevOps monitoring stacks to improve operational reliability and agility. From enriching and aggregating events to
correlating them into incidents, PagerDuty streamlines the incident management process by reducing alert noise and
resolution times.

## Prerequisites

You will need:

- an installation of the [PagerDuty agent](https://www.pagerduty.com/docs/guides/agent-install-guide/) on the node running the Netdata Agent
- a PagerDuty `Generic API` service using either the `Events API v2` or `Events API v1`
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to PagerDuty

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Firstly, [Add a new service](https://support.pagerduty.com/docs/services-and-integrations#section-configuring-services-and-integrations)
to PagerDuty. Click **Use our API directly** and select either `Events API v2` or `Events API v1`. Once you finish
creating the service, click on the **Integrations** tab to find your **Integration Key**.

Then, edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_PD` to `YES`.
2. Set `DEFAULT_RECIPIENT_PD` to the PagerDuty service key you want the alert notifications to be sent to.  
   You can define multiple service keys like this: `pd_service_key_1 pd_service_key_2`.  
   All roles will default to this variable if left unconfigured.
3. If you chose `Events API v2` during service setup on PagerDuty, change `USE_PD_VERSION` to `2`.

You can then have different PagerDuty service keys per **role**, by editing `DEFAULT_RECIPIENT_PD` with the service key you want, in the following entries at the bottom of the same file:

```conf
role_recipients_pd[sysadmin]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxa"
role_recipients_pd[domainadmin]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxb"
role_recipients_pd[dba]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxc"
role_recipients_pd[webmaster]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxd"
role_recipients_pd[proxyadmin]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxe"
role_recipients_pd[sitemgr]="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxf"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# pagerduty.com notification options

SEND_PD="YES"
DEFAULT_RECIPIENT_PD="xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
USE_PD_VERSION="2"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
