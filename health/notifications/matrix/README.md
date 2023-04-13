# Matrix Agent alert notifications

Learn how to send notifications to Matrix network rooms using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

## Prerequisites

You will need:

- The url of the homeserver (`https://homeserver:port`).
- Credentials for connecting to the homeserver, in the form of a valid access token for your account (or for a dedicated notification account). These tokens usually don't expire.
- The room ids that you want to sent the notification to.

## Configure Netdata to send alert notifications to Matrix

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_MATRIX` to `YES`.
2. Set `MATRIX_HOMESERVER` to the URL of the Matrix homeserver.
3. Set `MATRIX_ACCESSTOKEN` to the access token from your Matrix account.  
    To obtain the access token, you can use the following `curl` command:

    ```bash
    curl -XPOST -d '{"type":"m.login.password", "user":"example", "password":"wordpass"}' "https://homeserver:8448/_matrix/client/r0/login"
    ```

4. Set `DEFAULT_RECIPIENT_MATRIX` to the rooms you want the alert notifications to be sent to.  
    The format is `!roomid:homeservername`.  

    The room ids are unique identifiers and can be obtained from the room settings in a Matrix client (e.g. Riot).

    You can define multiple rooms like this: `!roomid1:homeservername !roomid2:homeservername`.  
    All roles will default to this variable if left unconfigured.

Detailed information about the Matrix client API is available at the [official site](https://matrix.org/docs/guides/client-server.html).

You can then have different rooms per **role**, by editing `DEFAULT_RECIPIENT_MATRIX` with the `!roomid:homeservername` you want, in the following entries at the bottom of the same file:

```conf
role_recipients_matrix[sysadmin]="!roomid1:homeservername"
role_recipients_matrix[domainadmin]="!roomid2:homeservername"
role_recipients_matrix[dba]="!roomid3:homeservername"
role_recipients_matrix[webmaster]="!roomid4:homeservername"
role_recipients_matrix[proxyadmin]="!roomid5:homeservername"
role_recipients_matrix[sitemgr]="!roomid6:homeservername"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Matrix notifications

SEND_MATRIX="YES"
MATRIX_HOMESERVER="https://matrix.org:8448"
MATRIX_ACCESSTOKEN="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
DEFAULT_RECIPIENT_MATRIX="!XXXXXXXXXXXX:matrix.org"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
