<!--
title: "Dynatrace"
sidebar_label: "Dynatrace Events"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/dynatrace/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Dynatrace

Dynatrace allows you to receive notifications using their Events REST API.

See [the Dynatrace documentation](https://www.dynatrace.com/support/help/extend-dynatrace/dynatrace-api/environment-api/events/post-event/) about POSTing an event in the Events API for more details.



You need:

1.  Dynatrace Server. You can use the same on all your Netdata servers but make sure the server is network visible from your Netdata hosts.
The Dynatrace server should be with protocol prefixed (`http://` or `https://`). For example: `https://monitor.example.com`
This is a required parameter.
2.  API Token. Generate a secure access API token that enables access to your Dynatrace monitoring data via the REST-based API.
Generate a Dynatrace API authentication token. On your Dynatrace server, go to **Settings** --> **Integration** --> **Dynatrace API** --> **Generate token**.
See [Dynatrace API - Authentication](https://www.dynatrace.com/support/help/extend-dynatrace/dynatrace-api/basics/dynatrace-api-authentication/) for more details.
This is a required parameter.
3.  API Space. This is the URL part of the page you have access in order to generate the API Token. For example, the URL
    for a generated API token might look like:
    `https://monitor.illumineit.com/e/2a93fe0e-4cd5-469a-9d0d-1a064235cfce/#settings/integration/apikeys;gf=all` In that
    case, my space is _2a93fe0e-4cd5-469a-9d0d-1a064235cfce_ This is a required parameter.
4. Generate a Server Tag. On your Dynatrace Server, go to **Settings** --> **Tags** --> **Manually applied tags** and create the Tag.
The Netdata alarm is sent as a Dynatrace Event to be correlated with all those hosts tagged with this Tag you have created.
This is a required parameter.
5. Specify the Dynatrace event. This can be one of `CUSTOM_INFO`, `CUSTOM_ANNOTATION`, `CUSTOM_CONFIGURATION`, and `CUSTOM_DEPLOYMENT`. 
The default value is `CUSTOM_INFO`.
This is a required parameter.
6. Specify the annotation type. This is the source of the Dynatrace event. Put whatever it fits you, for example, 
_Netdata Alarm_, which is also the default value.
