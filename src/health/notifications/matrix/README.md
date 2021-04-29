<!--
title: "Send Netdata notifications to Matrix network rooms"
description: "Stay aware of warning or critical anomalies by sending health alarms to Matrix network rooms with Netdata's health monitoring watchdog."
sidebar_label: "Matrix"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/matrix/README.md
-->

# Matrix

Send notifications to [Matrix](https://matrix.org/) network rooms.

The requirements for this notification method are:

1.  The url of the homeserver (`https://homeserver:port`).
2.  Credentials for connecting to the homeserver, in the form of a valid access token for your account (or for a
    dedicated notification account). These tokens usually don't expire.
3.  The room ids that you want to sent the notification to.

To obtain the access token, you can use the following `curl` command:

```bash
curl -XPOST -d '{"type":"m.login.password", "user":"example", "password":"wordpass"}' "https://homeserver:8448/_matrix/client/r0/login"
```

The room ids are unique identifiers and can be obtained from the room settings in a Matrix client (e.g. Riot). Their
format is `!uniqueid:homeserver`.

Multiple room ids can be defined by separating with a space character.

Detailed information about the Matrix client API is available at the [official
site](https://matrix.org/docs/guides/client-server.html).

Your `health_alarm_notify.conf` should look like this:

```conf
###############################################################################
# Matrix notifications
#

# enable/disable Matrix notifications
SEND_MATRIX="YES"

# The url of the Matrix homeserver
# e.g https://matrix.org:8448
MATRIX_HOMESERVER="https://matrix.org:8448"

# A access token from a valid Matrix account. Tokens usually don't expire,
# can be controlled from a Matrix client.
# See https://matrix.org/docs/guides/client-server.html
MATRIX_ACCESSTOKEN="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# Specify the default rooms to receive the notification if no rooms are provided
# in a role's recipients.
# The format is !roomid:homeservername
DEFAULT_RECIPIENT_MATRIX="!XXXXXXXXXXXX:matrix.org"
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fmatrix%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
