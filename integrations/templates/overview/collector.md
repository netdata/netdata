# [[ entry.meta.monitored_instance.name ]]

Plugin: [[ entry.meta.plugin_name ]]
Module: [[ entry.meta.module_name ]]

{% if entry.meta.keywords %}
Keywords: [[ entry.meta.keywords | join(',  ') ]]
{% endif %}

## Overview

[[ entry.overview.data_collection.metrics_description ]]

[[ entry.overview.data_collection.method_description ]]

[% if entry.overview.supported_platforms.include %]
This collector is only supported on the following platforms:

[% for platform in entry.overview.supported_platforms.include %]
- [[ platform ]]
[% endfor %]
[% elif entry.overview.supported_platforms.exclude %]
This collector is supported on all platforms except for the following platforms:

[% for platform in entry.overview.supported_platforms.exclude %]
- [[ platform ]]
[% endfor %]
[% else %]
This collector is supported on all platforms.
[% endif %]

[% if entry.overview.multi_instance %]
This collector supports collecting metrics from multiple instances of this integration, including remote instances.
[% else %]
This collector only supports collecting metrics from a single instance of this integration.
[% endif %]

[% if entry.overview.additional_permissions.description %]
[[ entry.overview.additional_permissions.description ]]
[% endif %]

[% if related %]
[[ entry.meta.name ]] can be monitored further using the following other integrations:

[% for res in related %]
- {% relatedResource id="[[ res.id ]]" %}[[ res.name ]]{% /relatedResource %}
[% endfor %]

[% endif %]
### Default Behavior

#### Auto-Detection

[% if entry.overview.default_behavior.auto_detection.description %]
[[ entry.overview.default_behavior.auto_detection.description ]]
[% else %]
This integration doesn't support auto-detection.
[% endif %]

#### Limits

[% if entry.overview.default_behavior.limits.description %]
[[ entry.overview.default_behavior.limits.description ]]
[% else %]
The default configuration for this integration does not impose any limits on data collection.
[% endif %]

#### Performance Impact

[% if entry.overview.default_behavior.performance_impact.description %]
[[ entry.overview.default_behavior.performance_impact.description ]]
[% else %]
The default configuration for this integration is not expected to impose a significant performance impact on the system.
[% endif %]
