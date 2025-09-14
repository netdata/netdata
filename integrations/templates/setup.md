## Setup
[% if entry.integration_type == 'logs' %]

## Prerequisites

[[ entry.setup.prerequisites.description]]

## Configuration

There is no configuration needed for this integration.
[% else %]

[% if entry.setup.description %]
[[ entry.setup.description ]]
[% else %]
### Prerequisites
[% if entry.setup.prerequisites.list %]

[% for prereq in entry.setup.prerequisites.list %]
#### [[ prereq.title ]]

[[ prereq.description ]]

[% endfor %]

[% else %]

No action required.

[% endif %]
### Configuration

#### Options

[[ entry.setup.configuration.options.description ]]

[% if entry.setup.configuration.options.list %]
[% if entry.setup.configuration.options.folding.enabled and not clean %]
{% details open=true summary="[[ entry.setup.configuration.options.folding.title ]]" %}
[% endif %]
| Name | Description | Default | Required |
|:----|:-----------|:-------|:--------:|
[% for item in entry.setup.configuration.options.list %]
| [[ strfy(item.name) ]] | [[ strfy(item.description) ]] | [[ strfy(item.default_value) ]] | [[ strfy(item.required) ]] |
[% endfor %]

[% for item in entry.setup.configuration.options.list %]
[% if 'detailed_description' in item %]
##### [[ item.name ]]

[[ item.detailed_description ]]

[% endif %]
[% endfor %]
[% if entry.setup.configuration.options.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% elif not entry.setup.configuration.options.description %]
There are no configuration options.

[% endif %]

[% if entry.meta.plugin_name == 'go.d.plugin' %]
#### via UI

The **[[ entry.meta.module_name ]]** collector can be configured directly through the Netdata web interface:

1. Go to **Nodes**.
2. Select the node **where you want the [[ entry.meta.module_name ]] data-collection job to run** and click the :gear: (**Configure this node**). This node will be responsible for collecting metrics.
3. The **Collectors → Jobs** view opens by default.
4. In the Search box, type [[ entry.meta.module_name ]] (or scroll the list) to locate the **[[ entry.meta.module_name ]]** collector.
5. Click the **+** next to the **[[ entry.meta.module_name ]]** collector to add a new job.
6. Fill in the job fields, then **Test** the configuration and **Submit**.

[% endif %]

#### via File

[% if entry.setup.configuration.file.name %]
The configuration file name for this integration is `[[ entry.setup.configuration.file.name ]]`.
[% if 'section_name' in entry.setup.configuration.file %]
Configuration for this specific integration is located in the `[[ entry.setup.configuration.file.section_name ]]` section within that file.
[% endif %]

[% if entry.meta.plugin_name == 'go.d.plugin' %]
[% include 'setup/sample-go-config.md' %]
[% elif entry.meta.plugin_name == 'python.d.plugin' %]
[% include 'setup/sample-python-config.md' %]
[% elif entry.meta.plugin_name == 'charts.d.plugin' %]
[% include 'setup/sample-charts-config.md' %]
[% elif entry.meta.plugin_name == 'ioping.plugin' %]
[% include 'setup/sample-charts-config.md' %]
[% elif entry.meta.plugin_name == 'apps.plugin' %]
[% include 'setup/sample-apps-config.md' %]
[% elif entry.meta.plugin_name == 'ebpf.plugin' %]
[% include 'setup/sample-netdata-config.md' %]
[% elif entry.setup.configuration.file.name == 'netdata.conf' %]
[% include 'setup/sample-netdata-config.md' %]
[% endif %]

You can edit the configuration file using the [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script from the
Netdata [config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config [[ entry.setup.configuration.file.name ]]
```
[% else %]
There is no configuration file.
[% endif %]

##### Examples
[% if entry.setup.configuration.examples.list %]

[% for example in entry.setup.configuration.examples.list %]
###### [[ example.name ]]

[[ example.description ]]

[% if example.folding.enabled and not clean %]
{% details open=true summary="[[ entry.setup.configuration.examples.folding.title ]]" %}
[% endif %]
```yaml
[[ example.config ]]
```
[% if example.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% endfor %]
[% else%]
There are no configuration examples.

[% endif %]
[% endif %]
[% endif %]