[# Jinja template: integrations/templates/setup-service_discovery.md
   Closely mirrors setup-secretstore.md but for SD discoverers.
   The 'via File' section explicitly notes the dual-block (`discoverer:` + `services:`)
   structure and points readers forward to the Service Rules section. #]
## Setup

You can configure the `[[ entry.meta.kind ]]` discoverer in two ways:

| Method | Best for | How to |
|:--|:--|:--|
| [**UI**](#via-ui) | Fast setup without editing files | Go to `Collectors -> go.d -> ServiceDiscovery -> [[ entry.meta.kind ]]`, then add a discovery pipeline. |
| [**File**](#via-file) | File-based configuration or automation | Edit `/etc/netdata/[[ entry.setup.configuration.file.name ]]` and define the `discoverer:` and `services:` blocks. |

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
{% details open=true summary="[[ entry.setup.configuration.options.folding.title or 'Discoverer options' ]]" %}
[% endif %]

| Option | Description | Default | Required |
|:-----|:------------|:--------|:---------:|
[% for item in entry.setup.configuration.options.list %]
[% set item_anchor = "option-" ~ anchorfy(item.name) %]
| [[ ("[" ~ strfy(item.name) ~ "](#" ~ item_anchor ~ ")") if ('detailed_description' in item) else strfy(item.name) ]] | [[ strfy(item.description) ]] | [[ strfy(item.default_value) ]] | [[ strfy(item.required) ]] |
[% endfor %]

[% for item in entry.setup.configuration.options.list %]
[% if 'detailed_description' in item %]
<a id="[[ "option-" ~ anchorfy(item.name) ]]"></a>
##### [[ item.name ]]

[[ item.detailed_description ]]

[% endif %]
[% endfor %]

[% if entry.setup.configuration.options.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% else %]
There are no configuration options.

[% endif %]

#### via UI

1. Open the Netdata Dynamic Configuration UI.
2. Go to `Collectors -> go.d -> ServiceDiscovery -> [[ entry.meta.kind ]]`.
3. Add a new discovery pipeline and give it a name.
4. Fill in the discoverer-specific settings and the service rules.
5. Save the discovery pipeline.

#### via File

Define the discovery pipeline in `/etc/netdata/[[ entry.setup.configuration.file.name ]]`.

The file has two top-level blocks: `discoverer:` (the options above) and `services:` (rules that turn discovered targets into collector jobs — see [Service Rules](#service-rules)).

After editing the file, restart the Netdata Agent to load the updated discovery pipeline.

##### Examples
[% if entry.setup.configuration.examples.list %]

[% for example in entry.setup.configuration.examples.list %]
###### [[ example.name ]]

[[ example.description ]]

[% if example.folding is defined and example.folding.enabled and not clean %]
{% details open=true summary="[[ entry.setup.configuration.examples.folding.title or 'Example configuration' ]]" %}
[% endif %]
```yaml
[[ example.config ]]
```
[% if example.folding is defined and example.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% endfor %]
[% else %]
There are no configuration examples.

[% endif %]
