<!--
---
title: "Dynatrace"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/dynatrace/README.md
---
-->

# Dynatrace

Dynatrace allows you to receive notification using their Events REST API.

For more details, see [Events API - POST an event](https://www.dynatrace.com/support/help/extend-dynatrace/dynatrace-api/environment-api/events/post-event/).



You need:

1.  Dynatrace Server. You can use the same on all your Netdata servers but make sure the server is network visible from your Netdata hosts.
The Dynatrace server should be with protocol prefixed (http:// or https://), example https://monitor.illumineit.com
This is a required parameter.
2.  API Token. Generate a secure access API token that enables access to your Dynatrace monitoring data via the REST-based API.
Generate a Dynatrace API authentication token. On your Dynatrace server goto Settings --> Integration --> Dynatrace API --> Generate token
For more details, see [Dynatrace API - Authentication](https://www.dynatrace.com/support/help/extend-dynatrace/dynatrace-api/basics/dynatrace-api-authentication/).
This is a required parameter.
3.  The API Space. This is the URL part of the page you have access in order to generate the API Token. For example, for my generated API Token the URL is:
https://monitor.illumineit.com/e/2a93fe0e-4cd5-469a-9d0d-1a064235cfce/#settings/integration/apikeys;gf=all
In that case, my space is _2a93fe0e-4cd5-469a-9d0d-1a064235cfce_
This is a required parameter.
4. Generate a Server Tag. On your Dynatrace Server go to Settings --> Tags --> Manually applied tags and create the Tag
The NetData alarm will be sent as a Dynatrace Event to be correlated with all those hosts tagged with this Tag you have created.
This is a required parameter.
5. Specify the Dynatrace event. This can be one of CUSTOM_INFO, CUSTOM_ANNOTATION, CUSTOM_CONFIGURATION, CUSTOM_DEPLOYMENT
The default value is CUSTOM_INFO
This is a required parameter.
6. Specify the annotation type. Practically this is the source of the Dynatrace event. Put whatever it fits you, for example 
_NetData Alarm_ that is also the default value.
