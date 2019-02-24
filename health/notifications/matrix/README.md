# matrix.org - riot.im

Send notifications to matrix network rooms.

The requirements for this connector are
1. The url of the homeserver (https://homeserver:port)
2. Credentials for connecting to the homeserver, in the form of a valid accesstoken for your account (or for a dedicated notification account). These tokens usually don't expire.
3. The room ids that you want to sent the notification to.

To obtain the access token you can use the following curl command:
```
curl -XPOST -d '{"type":"m.login.password", "user":"example", "password":"wordpass"}' "https://homeserver:8448/_matrix/client/r0/login"
```

The room ids are unique identifiers and can be obtained from the room settings in a matrix client (riot). Their format is !uniqueid:homeserver.
Multiple room ids can be defined by separating with a space character.

Detailed information about the matrix client api is available at the official [site](https://matrix.org/docs/guides/client-server.html)

Your health_alarm_notify.conf should look like this:
```
###############################################################################
# Matrix notifications
#

# enable/disable matrix notifications
SEND_MATRIX="YES"

# The url of the matrix homeserver
# e.g https://matrix.org:8448
MATRIX_HOMESERVER="https://matrix.org:8448"

# A accesstoken from a valid matrix account. Tokens usually don't expire,
# can be controlled from a matrix client.
# See https://matrix.org/docs/guides/client-server.html
MATRIX_ACCESSTOKEN="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# Specify the default rooms to receive the notification if no rooms are provided
# in a role's recipients.
# The format is !roomid:homeservername
DEFAULT_RECIPIENT_MATRIX="!XXXXXXXXXXXX:matrix.org"
```
