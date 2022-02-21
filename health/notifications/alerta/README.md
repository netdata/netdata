<!--
title: "alerta.io"
description: "Send alarm notifications to Alerta to see the latest health status updates from multiple nodes in a single interface."
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/alerta/README.md
-->

# alerta.io

The [Alerta](https://alerta.io) monitoring system is a tool used to
consolidate and de-duplicate alerts from multiple sources for quick
‘at-a-glance’ visualisation. With just one system you can monitor
alerts from many other monitoring tools on a single screen.

![Alerta dashboard](https://docs.alerta.io/_images/alerta-screen-shot-3.png "Alerta dashboard showing several alerts.")

Alerta's advantage is the main view, where you can see all active alarms with the most recent state. You can also view an alert history. You can send Netdata alerts to Alerta to see alerts coming from many Netdata hosts or also from a multi-host
Netdata configuration. 

## Deploying Alerta

The recommended setup is using a dedicated server, VM or container. If you have other NGINX or Apache servers in your organization,
it is recommended to proxy to this new server.

You can install Alerta in several ways:
- **Docker**: Alerta provides a [Docker image](https://hub.docker.com/r/alerta/alerta-web/) to get you started quickly.
- **Deployment on Ubuntu server**: Alerta's [getting started tutorial](https://docs.alerta.io/gettingstarted/tutorial-1-deploy-alerta.html) walks you through this process. 
- **Advanced deployment scenarios**: More ways to install and deploy Alerta are documented on the [Alerta docs](http://docs.alerta.io/en/latest/deployment.html).

## Sending alerts to Alerta

### Step 1. Create an API key (if authentication in Alerta is enabled)

You will need an API key to send messages from any source, if
Alerta is configured to use authentication (recommended). 

Create a new API key in Alerta: 
1. Go to *Configuration* > *API Keys* 
2. Create a new API key called "netdata" with `write:alerts` permission.

### Step 2. Configure Netdata to send alerts to Alerta
1. Edit the `health_alarm_notify.conf` by running:
```sh
/etc/netdata/edit-config health_alarm_notify.conf
```

2. Modify the file as below:
```
# enable/disable sending alerta notifications
SEND_ALERTA="YES"

# here set your alerta server API url
# this is the API url you defined when installed Alerta server, 
# it is the same for all users. Do not include last slash.
ALERTA_WEBHOOK_URL="http://yourserver/alerta/api"

# Login with an administrative user to you Alerta server and create an API KEY
# with write permissions.
ALERTA_API_KEY="INSERT_YOUR_API_KEY_HERE"

# you can define environments in /etc/alertad.conf option ALLOWED_ENVIRONMENTS
# standard environments are Production and Development
# if a role's recipients are not configured, a notification will be send to
# this Environment (empty = do not send a notification for unconfigured roles):
DEFAULT_RECIPIENT_ALERTA="Production"
```

## Test alarms

We can test alarms using the standard approach:

```sh
/opt/netdata/netdata-plugins/plugins.d/alarm-notify.sh test
```

> **Note** This script will send 3 alarms. 
> Alerta will not show the alerts in the main page, because last alarm is "CLEAR".
> To see the test alarms, you need to select "closed" alarms in the top-right lookup. 

For more information see the [Alerta documentation](https://docs.alerta.io)


