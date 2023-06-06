# Add webhook notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on a webhook using a predefined schema.

## Prerequisites

To add webhook notification configurations you need:

- A Netdata Cloud account
- Access to the space as an **administrator**
- Space needs to be on **Pro** plan or higher
- Have an app that allows you to receive webhooks following a predefined schema, for more details check [how to create the webhook service](#webhook-service)

## Steps

1. Click on the **Space settings** cog (located above your profile icon)
1. Click on the **Notification** tab
1. Click on the **+ Add configuration** button (near the top-right corner of your screen)
1. On the **webhook** card click on **+ Add**
1. A modal will be presented to you to enter the required details to enable the configuration:
   1. **Notification settings** are Netdata specific settings
      - Configuration name - you can optionally provide a name for your configuration  you can easily refer to it
      - Rooms - by specifying a list of Rooms you are select to which nodes or areas of your infrastructure you want to be notified using this configuration
      - Notification - you specify which notifications you want to be notified using this configuration: All Alerts and unreachable, All Alerts, Critical only
   1. **Integration configuration** are the specific notification integration required settings, which vary by notification method. For webhook:
      - Webhook URL - webhook URL is the url of the service that Netdata will send notifications to. In order to keep the communication secured, we only accept HTTPS urls. Check [how to create the webhook service](#webhook-service).
      - Extra headers - these are optional key-value pairs that you can set to be included in the HTTP requests sent to the webhook URL. For more details check [Extra headers](#extra-headers)
      - Authentication Mechanism - Netdata webhook integration supports 3 different authentication mechanisms. For more details check [Authentication mechanisms](#authentication-mechanisms):
         - Mutual TLS (recommended) - default authentication mechanism used if no other method is selected.
         - Basic - the client sends a request with an Authorization header that includes a base64-encoded string in the format **username:password**. These will settings will be required inputs.
         - Bearer - the client sends a request with an Authorization header that includes a **bearer token**. This setting will be a required input.

## Webhook service

A webhook integration allows your application to receive real-time alerts from Netdata by sending HTTP requests to a specified URL. In this document, we'll go over the steps to set up a generic webhook integration, including adding headers, and implementing different types of authorization mechanisms.

### Netdata webhook integration

A webhook integration is a way for one service to notify another service about events that occur within it. This is done by sending an HTTP POST request to a specified URL (known as the "webhook URL") when an event occurs.

Netdata webhook integration service will send alert notifications to the destination service as soon as they are detected.

The notification content sent to the destination service will be a JSON object having these properties:

| field   | type   | description |
| :--     | :--    | :--         |
| message | string | A summary message of the alert. |
| alarm | string | The alarm the notification is about. |
| info | string | Additional info related with the alert. |
| chart | string | The chart associated with the alert. |
| context | string | The chart context. |
| space | string | The space where the node that raised the alert is assigned. |
| rooms | object[object(string,string)] | Object with list of rooms names and urls where the node belongs to. |
| family | string | Context family. |
| class | string | Classification of the alert, e.g. "Error". |
| severity | string | Alert severity, can be one of "warning", "critical" or "clear". |
| date | string | Date of the alert in ISO8601 format. |
| duration | string |  Duration the alert has been raised. |
| additional_active_critical_alerts | integer | Number of additional critical alerts currently existing on the same node. |
| additional_active_warning_alerts | integer | Number of additional warning alerts currently existing on the same node. |
| alarm_url | string | Netdata Cloud URL for this alarm. |

### Extra headers

When setting up a webhook integration, the user can specify a set of headers to be included in the HTTP requests sent to the webhook URL.

By default, the following headers will be sent in the HTTP request

|            **Header**            | **Value**                 |
|:-------------------------------:|-----------------------------|
|     Content-Type             | application/json        |

### Authentication mechanisms

Netdata webhook integration supports 3 different authentication mechanisms:

#### Mutual TLS authentication (recommended)

In mutual Transport Layer Security (mTLS) authentication, the client and the server authenticate each other using X.509 certificates. This ensures that the client is connecting to the intended server, and that the server is only accepting connections from authorized clients.

This is the default authentication mechanism used if no other method is selected.

To take advantage of mutual TLS, you can configure your server to verify Netdata's client certificate. In order to achieve this, the Netdata client sending the notification supports mutual TLS (mTLS) to identify itself with a client certificate that your server can validate.

The steps to perform this validation are as follows:

- Store Netdata CA certificate on a file in your disk. The content of this file should be:

<details>
  <summary>Netdata CA certificate</summary>

```
-----BEGIN CERTIFICATE-----
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
5zrbwvQf
-----END CERTIFICATE-----
```
</details>

- Enable client certificate validation on the web server that is doing the TLS termination. Below we show you how to perform this configuration in `NGINX` and `Apache`

 **NGINX**

```bash
server {
    listen 443 ssl default_server;

    # ... existing SSL configuration for server authentication ...
    ssl_verify_client on;
    ssl_client_certificate /path/to/Netdata_CA.pem;

    location / {
        if ($ssl_client_s_dn !~ "CN=api.netdata.cloud") {
            return 403;
        }
       # ... existing location configuration ...
    }
}
```

**Apache**

```bash
Listen 443
<VirtualHost *:443>
    # ... existing SSL configuration for server authentication ...
    SSLVerifyClient require
    SSLCACertificateFile "/path/to/Netdata_CA.pem"
</VirtualHost>
<Directory /var/www/>
    Require expr "%{SSL_CLIENT_S_DN_CN} == 'api.netdata.cloud'"
    # ... existing directory configuration ...
</Directory>
```

#### Basic authentication

In basic authorization, the client sends a request with an Authorization header that includes a base64-encoded string in the format username:password. The server then uses this information to authenticate the client. If this authentication method is selected, the user can set the user and password that will be used when connecting to the destination service.

#### Bearer token authentication

In bearer token authentication, the client sends a request with an Authorization header that includes a bearer token. The server then uses this token to authenticate the client. Bearer tokens are typically generated by an authentication service, and are passed to the client after a successful authentication. If this method is selected, the user can set the token to be used for connecting to the destination service.

##### Challenge secret

To validate that you has ownership of the web application that will receive the webhook events, we are using a challenge response check mechanism.

This mechanism works as follows:

- The challenge secret parameter that you provide is a shared secret between you and Netdata only.
- On your request for creating a new Webhook integration, we will make a GET request to the url of the webhook, adding a query parameter `crc_token`, consisting of a random string.
- You will receive this request on your application and it must construct an encrypted response, consisting of a base64-encoded HMAC SHA-256 hash created from the crc_token and the shared secret. The response will be in the format:

```json
{
  "response_token": "sha256=9GKoHJYmcHIkhD+C182QWN79YBd+D+Vkj4snmZrfNi4="
}
```

- We will compare your application's response with the hash that we will generate using the challenge secret, and if they are the same, the integration creation will succeed.

We will do this validation everytime you update your integration configuration.

- Response requirements:
    - A base64 encoded HMAC SHA-256 hash created from the crc_token and the shared secret.
    - Valid response_token and JSON format.
    - Latency less than 5 seconds.
    - 200 HTTP response code.

**Example response token generation in Python:**

Here you can see how to define a handler for a Flask application in python 3:

```python
import base64
import hashlib
import hmac
import json

key ='YOUR_CHALLENGE_SECRET'

@app.route('/webhooks/netdata')
def webhook_challenge():
  token = request.args.get('crc_token').encode('ascii')

  # creates HMAC SHA-256 hash from incomming token and your consumer secret
  sha256_hash_digest = hmac.new(key.encode(),
                                msg=token,
                                digestmod=hashlib.sha256).digest()

  # construct response data with base64 encoded hash
  response = {
    'response_token': 'sha256=' + base64.b64encode(sha256_hash_digest).decode('ascii')
  }

  # returns properly formatted json response
  return json.dumps(response)
```

#### Related topics

- [Alerts Configuration](https://github.com/netdata/netdata/blob/master/health/README.md)
- [Alert Notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
- [Manage notification methods](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md)
