# matrix.org - riot.im

Send notifications to matrix network rooms.

The requirements for this connector are
1) The url of the homeserver (https://homeserver:port)
1) Credentials for connecting to the homeserver, either:
  a) A valid accesstoken for your account (or a dedicated notification account). These tokens usually don't expire.
  b) A userid and password
2) The room ids that you want to sent the notification to.

To obtain the access token you can use the following curl command:
```
##############################################################################
curl -XPOST -d '{"type":"m.login.password", "user":"example", "password":"wordpass"}' "https://homeserver:8448/_matrix/client/r0/login"
```

The room ids are unique identifiers and can be obtained from the room settings in a matrix client (riot). Multiple room ids can be defined by separating with a space character.

Detailed information in the matrix client-server api https://matrix.org/docs/guides/client-server.html

```
###############################################################################
# how to send matrix notifications

# enable matrix notifications
SEND_MATRIX="YES"
```
