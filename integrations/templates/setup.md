[% if entry.setup.prerequisites.list %]
### Prerequisites

[% for prereq in entry.setup.prerequisites.list %]
#### [[ prereq.title ]]

[[ prereq.description ]]

[% endfor %]
[% endif %]
[% if entry.setup.configuration.file.name %]
### Configuration

#### File

The configuration file name for this integration is `[[ entry.setup.configuration.file.name ]]`.
[% if 'section_name' in entry.setup.configuration.file %]
Configuration for this specific integration is located in the `[[ entry.setup.configuration.file.section_name ]]` section within that file.
[% endif %]

The file format is YAML. Generally, the format is:

```yaml
update_every: 1
autodetection_retry: 0
jobs:
  - name: some_name1
  - name: some_name1
```

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config [[ entry.setup.configuration.file.name ]]
```

#### Options

[[ entry.setup.configuration.options.description ]]

[% if entry.setup.configuration.options.list %]
[% if entry.setup.configuration.options.folding.enabled %]
{% details summary="[[ entry.setup.configuration.options.folding.title ]]" %}
[% endif %]
| Name | Description | Default | Required |
|:----:|-------------|:-------:|:--------:|
[% for item in entry.setup.configuration.options.list %]
| [[ item.name ]] | [[ item.description ]] | [[ item.default ]] | [[ item.required ]] |
[% endfor %]
[% if entry.setup.configuration.options.folding.enabled %]
{% /details %}
[% endif %]
[% endif %]

[% if entry.setup.configuration.examples.list %]
#### Examples

[% for example in entry.setup.configuration.examples.list %]
##### [[ example.name ]]

[[ example.description ]]

[% if entry.setup.configuration.examples.folding.enabled %]
[% if example.folding %]
{% details summary="[[ entry.setup.configuration.examples.folding.title ]]" %}
[% endif %]
[% endif %]
```yaml
[[ example.config ]]
```
[% if entry.setup.configuration.examples.folding.enabled %]
[% if example.folding %]
{% /details %}
[% endif %]
[% endif %]
[% endfor %]
[% endif %]
[% endif %]
