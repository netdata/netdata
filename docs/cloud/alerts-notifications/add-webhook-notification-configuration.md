# Add webhook notification configuration

From the Cloud interface, you can manage your space's notification settings and from these you can add a specific configuration to get notifications delivered on a webhook using a predefined schema.

## Prerequisites

To add discord notification configurations you need

- A Netdata Cloud account
- Access to the space as an **administrator**
- Space needs to be on **Pro** plan or higher
- Have an app that allows you to receive webhooks following a predefined schema, for mode details check [how to create the webhook service](#webhook-service)

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
      - Extra headers - these are optional key-value pairs that you can set to be included in the HTTP requests sent to the webhook URL. For mode details check [Extra headers](#extra-headers)
      - Authorization Mechanism - Netdata webhook integration supports 3 different authorization mechanisms. For mode details check [Authorization mechanism](#authorization-mechanism):
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
| family | string | Context family. |
| class | string | Classification of the alert, e.g. "Error". |
| severity | string | Alert severity, can be one of "warning", "critical" or "clear". |
| date | string | Date of the alert in ISO8601 format. |
| duration | string |  Duration the alert has been raised. |
| critical_count | integer | umber of critical alerts currently existing on the same node. |
| warning_count | integer | Number of warning alerts currently existing on the same node. |
| alarm_url | string | Netdata Cloud URL for this alarm. |

### Extra headers

When setting up a webhook integration, the user can specify a set of headers to be included in the HTTP requests sent to the webhook URL.

By default, the following headers will be sent in the HTTP request

|            **Header**            | **Value**                 |
|:-------------------------------:|-----------------------------|
|     Content-Type             | application/json        |

### Authorization mechanism

Netdata webhook integration supports 3 different authorization mechanisms:

1. Mutual TLS (recommended)

In mutual Transport Layer Security (mTLS) authorization, the client and the server authenticate each other using X.509 certificates. This ensures that the client is connecting to the intended server, and that the server is only accepting connections from authorized clients.

To take advantage of mutual TLS, you can configure your server to verify Netdata's client certificate. To do that you need to download our [CA certificate file](http://localhost) and configure your server to use it as the

This is the default authentication mechanism used if no other method is selected.

2. Basic

In basic authorization, the client sends a request with an Authorization header that includes a base64-encoded string in the format username:password. The server then uses this information to authenticate the client. If this authentication method is selected, the user can set the user and password that will be used when connecting to the destination service.

3. Bearer

In bearer token authorization, the client sends a request with an Authorization header that includes a bearer token. The server then uses this token to authenticate the client. Bearer tokens are typically generated by an authentication service, and are passed to the client after a successful authentication. If this method is selected, the user can set the token to be used for connecting to the destination service.
