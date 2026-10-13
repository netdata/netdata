# [[ entry.meta.monitored_instance.name ]]

Plugin: [[ entry.meta.plugin_name ]]
Module: [[ entry.meta.module_name ]]

## Overview

[[ entry.overview.data_collection.metrics_description ]]

[[ entry.overview.data_collection.method_description ]]

[% if entry.overview.supported_platforms.include %]
This integration is only supported on the following platforms:

[% for platform in entry.overview.supported_platforms.include %]
- [[ platform ]]
[% endfor %]
[% elif entry.overview.supported_platforms.exclude %]
This integration is supported on all platforms except for the following platforms:

[% for platform in entry.overview.supported_platforms.exclude %]
- [[ platform ]]
[% endfor %]
[% else %]
This integration is supported on all platforms.
[% endif %]

[% if entry.overview.multi_instance %]
This integration supports multiple instances configured side-by-side.
[% else %]
This integration runs as a single instance per Netdata Agent.
[% endif %]

[% if entry.overview.additional_permissions.description %]
[[ entry.overview.additional_permissions.description ]]
[% endif %]

[% if related %]
[[ entry.meta.monitored_instance.name ]] can be combined with the following other integrations:

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
The default configuration for this integration does not impose any limits.
[% endif %]

#### Performance Impact

[% if entry.overview.default_behavior.performance_impact.description %]
[[ entry.overview.default_behavior.performance_impact.description ]]
[% else %]
The default configuration for this integration is not expected to impose a significant performance impact on the system.
[% endif %]
