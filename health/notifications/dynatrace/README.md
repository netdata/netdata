# Dynatrace Agent alert notifications

Learn how to send notifications to Dynatrace using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

Dynatrace allows you to receive notifications using their Events REST API.

See [the Dynatrace documentation](https://www.dynatrace.com/support/help/dynatrace-api/environment-api/events-v2/post-event) about POSTing an event in the Events API for more details.

## Prerequisites

You will need:

- A Dynatrace Server. You can use the same on all your Netdata servers but make sure the server is network visible from your Netdata hosts.
  The Dynatrace server should be with protocol prefixed (`http://` or `https://`), for example: `https://monitor.example.com`.
- An API Token. Generate a secure access API token that enables access to your Dynatrace monitoring data via the REST-based API.  
  See [Dynatrace API - Authentication](https://www.dynatrace.com/support/help/extend-dynatrace/dynatrace-api/basics/dynatrace-api-authentication/) for more details.
- An API Space. This is the URL part of the page you have access in order to generate the API Token.  
  For example, the URL for a generated API token might look like: `https://monitor.illumineit.com/e/2a93fe0e-4cd5-469a-9d0d-1a064235cfce/#settings/integration/apikeys;gf=all` In that case, the Space is `2a93fe0e-4cd5-469a-9d0d-1a064235cfce`.
- A Server Tag. To generate one on your Dynatrace Server, go to **Settings** --> **Tags** --> **Manually applied tags** and create the Tag.
  The Netdata alarm is sent as a Dynatrace Event to be correlated with all those hosts tagged with this Tag you have created.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Dynatrace

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`:

1. Set `SEND_DYNATRACE` to `YES`.
2. Set `DYNATRACE_SERVER` to the Dynatrace server with the protocol prefix, for example `https://monitor.example.com`.
3. Set `DYNATRACE_TOKEN` to your Dynatrace API authentication token
4. Set `DYNATRACE_SPACE` to the API Space, it is the URL part of the page you have access in order to generate the API Token. For example, the URL for a generated API token might look like: `https://monitor.illumineit.com/e/2a93fe0e-4cd5-469a-9d0d-1a064235cfce/#settings/integration/apikeys;gf=all` In that case, the Space is `2a93fe0e-4cd5-469a-9d0d-1a064235cfce`.
5. Set `DYNATRACE_TAG_VALUE` to your Dynatrace Server Tag.
6. `DYNATRACE_ANNOTATION_TYPE` can be left to its default value `Netdata Alarm`, but you can change it to better fit  your needs.
7. Set `DYNATRACE_EVENT` to the Dynatrace `eventType` you want, possible values are:  
   `AVAILABILITY_EVENT`, `CUSTOM_ALERT`, `CUSTOM_ANNOTATION`, `CUSTOM_CONFIGURATION`, `CUSTOM_DEPLOYMENT`, `CUSTOM_INFO`, `ERROR_EVENT`, `MARKED_FOR_TERMINATION`, `PERFORMANCE_EVENT`, `RESOURCE_CONTENTION_EVENT`. You can read more [here](https://www.dynatrace.com/support/help/dynatrace-api/environment-api/events-v2/post-event#request-body-objects)

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Dynatrace global notification options

SEND_DYNATRACE="YES"
DYNATRACE_SERVER="https://monitor.example.com"
DYNATRACE_TOKEN="XXXXXXX"
DYNATRACE_SPACE="2a93fe0e-4cd5-469a-9d0d-1a064235cfce"
DYNATRACE_TAG_VALUE="SERVERTAG"
DYNATRACE_ANNOTATION_TYPE="Netdata Alert"
DYNATRACE_EVENT="AVAILABILITY_EVENT"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
