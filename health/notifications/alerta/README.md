# alerta.io

The [Alerta](https://alerta.io) monitoring system is a tool used to
consolidate and de-duplicate alerts from multiple sources for quick
‘at-a-glance’ visualisation. With just one system you can monitor
alerts from many other monitoring tools on a single screen.

![](https://docs.alerta.io/en/latest/_images/alerta-screen-shot-3.png)

Netadata alarms can be sent to Alerta so you can see in one place
alerts coming from many Netdata hosts or also from a multi-host
Netadata configuration. The big advantage over other notifications
systems is that there is a main view of all active alarms with
the most recent state, and it is also possible to view alarm history.

## Deploying Alerta

It is recommended to set up the server in a separated server, VM or
container. If you have other Nginx or Apache server in your organization,
it is recommended to proxy to this new server.

The easiest way to install Alerta is to use the Docker image available
on [Docker hub][1]. Alternatively, follow the ["getting started"][2]
tutorial to deploy Alerta to an Ubuntu server. More advanced
configurations are out os scope of this tutorial but information
about different deployment scenaries can be found in the  [docs][3].

[1]: https://hub.docker.com/r/alerta/alerta-web/

[2]: http://alerta.readthedocs.io/en/latest/gettingstarted/tutorial-1-deploy-alerta.html

[3]: http://docs.alerta.io/en/latest/deployment.html

## Send alarms to Alerta

Step 1. Create an API key (if authentication is enabled)

You will need an API key to send messages from any source, if
Alerta is configured to use authentication (recommended). To
create an API key go to "Configuration -> API Keys" and create
a new API key called "netdata" with `write:alerts` permission.

Step 2. configure Netdata to send alarms to Alerta

On your system run:

```sh
/etc/netdata/edit-config health_alarm_notify.conf
```

and modify the file as below:

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

Note: Netdata will send 3 alarms, and because last alarm is "CLEAR"
you will not see them in main Alerta page, you need to select to see
"closed" alarma in top-right lookup. A little change in `alarm-notify.sh`
that let us test each state one by one will be useful.

For more information see <https://docs.alerta.io>

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Falerta%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
