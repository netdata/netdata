# IRC notifications

This is what you will get:

IRCCloud web client:  
![image](https://user-images.githubusercontent.com/31221999/36793487-3735673e-1ca6-11e8-8880-d1d8b6cd3bc0.png)

Irssi terminal client:
![image](https://user-images.githubusercontent.com/31221999/36793486-3713ada6-1ca6-11e8-8c12-70d956ad801e.png)


You need:
1. The `nc` utility. If you do not set the path, netdata will search for it in your system `$PATH`.

Set the path for `nc` in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
#------------------------------------------------------------------------------
# external commands
#
# The full path of the nc command.
# If empty, the system $PATH will be searched for it.
# If not found, irc notifications will be silently disabled.
nc="/usr/bin/nc"

```

2. Αn `IRC_NETWORK` to which your preffered channels belong to.   
3. One or more channels ( `DEFAULT_RECIPIENT_IRC` ) to post the messages to.   
4. An `IRC_NICKNAME` and an `IRC_REALNAME` to identify in IRC.   

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
#------------------------------------------------------------------------------
# irc notification options
#
# irc notifications require only the nc utility to be installed. 

# multiple recipients can be given like this:
#              "<irc_channel_1> <irc_channel_2> ..."

# enable/disable sending irc notifications
SEND_IRC="YES"

# if a role's recipients are not configured, a notification will not be sent.
# (empty = do not send a notification for unconfigured roles):
DEFAULT_RECIPIENT_IRC="#system-alarms"

# The irc network to which the recipients belong. It must be the full network.
IRC_NETWORK="irc.freenode.net"

# The irc nickname which is required to send the notification. It must not be 
# an already registered name as the connection's MODE is defined as a 'guest'.
IRC_NICKNAME="netdata-alarm-user"

# The irc realname which is required in order to make the connection and is an
# extra identifier.
IRC_REALNAME="netdata-user"

```

You can define multiple channels like this: `#system-alarms #networking-alarms`.  
You can also filter the notifications like this: `#system-alarms|critical`.  
You can give different channels per **role** using these (at the same file):  

```
role_recipients_irc[sysadmin]="#user-alarms #networking-alarms #system-alarms"
role_recipients_irc[dba]="#databases-alarms"
role_recipients_irc[webmaster]="#networking-alarms"
```

The keywords `#user-alarms`, `#networking-alarms`, `#system-alarms`, `#databases-alarms` are irc channels which belong to the specified IRC network.