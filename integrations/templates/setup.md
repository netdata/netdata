## Setup

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

#### File

[% if entry.setup.configuration.file.name %]
The configuration file name for this integration is `[[ entry.setup.configuration.file.name ]]`.
[% if 'section_name' in entry.setup.configuration.file %]
Configuration for this specific integration is located in the `[[ entry.setup.configuration.file.section_name ]]` section within that file.
[% endif %]

[% if entry.plugin_name == 'go.d.plugin' %]
[% include 'setup/sample-go-config.md' %]
[% elif entry.plugin_name == 'python.d.plugin' %]
[% include 'setup/sample-python-config.md' %]
[% elif entry.plugin_name == 'charts.d.plugin' %]
[% include 'setup/sample-charts-config.md' %]
[% elif entry.plugin_name == 'ioping.plugin' %]
[% include 'setup/sample-charts-config.md' %]
[% elif entry.plugin_name == 'apps.plugin' %]
[% include 'setup/sample-apps-config.md' %]
[% elif entry.plugin_name == 'ebpf.plugin' %]
[% include 'setup/sample-netdata-config.md' %]
[% elif entry.setup.configuration.file.name == 'netdata.conf' %]
[% include 'setup/sample-netdata-config.md' %]
[% endif %]

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config [[ entry.setup.configuration.file.name ]]
```
[% else %]
There is no configuration file.
[% endif %]
#### Options

[[ entry.setup.configuration.options.description ]]

[% if entry.setup.configuration.options.list %]
[% if entry.setup.configuration.options.folding.enabled %]
{% details summary="[[ entry.setup.configuration.options.folding.title ]]" %}
[% endif %]
| Name | Description | Default | Required |
|:----|:-----------|:-------|:--------:|
[% for item in entry.setup.configuration.options.list %]
| [[ item.name ]] | [[ item.description ]] | [[ item.default ]] | [[ item.required ]] |
[% endfor %]

[% for item in entry.setup.configuration.options.list %]
[% if 'detailed_description' in item %]
##### [[ item.name ]]

[[ item.detailed_description ]]

[% endif %]
[% endfor %]
[% if entry.setup.configuration.options.folding.enabled %]
{% /details %}
[% endif %]
[% elif not entry.setup.configuration.options.description %]
There are no configuration options.

[% endif %]
#### Examples
[% if entry.setup.configuration.examples.list %]

[% for example in entry.setup.configuration.examples.list %]
##### [[ example.name ]]

[[ example.description ]]

[% if example.folding.enabled %]
{% details summary="[[ entry.setup.configuration.examples.folding.title ]]" %}
[% endif %]
```yaml
[[ example.config ]]
```
[% if example.folding.enabled %]
{% /details %}
[% endif %]
[% endfor %]
[% else%]
There are no configuration examples.

[% endif %]
[% endif %]
