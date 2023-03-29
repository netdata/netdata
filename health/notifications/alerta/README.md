# Alerta Agent alert notifications

Send alarm notifications to Alerta to see the latest health status updates from multiple nodes in a single interface.

> ### Note
>
> This file assumes you have read the [landing page of this section](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alerting method works.

The [Alerta](https://alerta.io) monitoring system is a tool used to consolidate and de-duplicate alerts from multiple sources for quick ‘at-a-glance’ visualization.
With just one system you can monitor alerts from many other monitoring tools on a single screen.

![Alerta dashboard](https://docs.alerta.io/_images/alerta-screen-shot-3.png "Alerta dashboard showing several alerts.")

Alerta's advantage is the main view, where you can see all active alarms with the most recent state.
You can also view an alert history. You can send Netdata alerts to Alerta to see alerts coming from many Netdata hosts or also from a multi-host Netdata configuration.

## Prerequisites

You will need to:

- Create an Alerta API key (if authentication in Alerta is enabled)
- Have terminal access to the Agent you wish to configure

## Deploying Alerta

The recommended setup is using a dedicated server, VM or container.
If you have other NGINX or Apache servers in your organization, it is recommended to proxy to this new server.

You can install Alerta in several ways:

- **Docker**: Alerta provides a [Docker image](https://hub.docker.com/r/alerta/alerta-web/) to get you started quickly.
- **Deployment on Ubuntu server**: Alerta's [getting started tutorial](https://docs.alerta.io/gettingstarted/tutorial-1-deploy-alerta.html) walks you through this process.
- **Advanced deployment scenarios**: More ways to install and deploy Alerta are documented on the [Alerta docs](http://docs.alerta.io/en/latest/deployment.html).

## Sending alerts to Alerta

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> - Please also note that after most configuration changes you will need to [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for the changes to take effect.
>
> It is recommended to use this way for configuring Netdata.

### Create an API key (if authentication in Alerta is enabled)

You will need an API key to send messages from any source, if
Alerta is configured to use authentication (recommended).

Create a new API key in Alerta:

1. Go to *Configuration* > *API Keys*
2. Create a new API key called "netdata" with `write:alerts` permission.

### Configure Netdata to send alerts to Alerta

Edit `health_alarm_notify.conf`:

1. Set `SEND_ALERTA` to `YES`
2. set `ALERTA_WEBHOOK_URL` to the API url you defined when you installed the Alerta server
3. Set `ALERTA_API_KEY` to the API key you created previously
4. Set `DEFAULT_RECIPIENT_ALERTA` to the default recipient environment you want to alert to  
   All roles will default to this environment if left unconfigured.

An example working configuration would be:

```conf
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

## Test the alert

To test this alert refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.

## More information

For more information see the [Alerta documentation](https://docs.alerta.io).
