## Setup

You can configure the `[[ entry.meta.kind ]]` secretstore in two ways:

| Method | Best for | How to |
|:--|:--|:--|
| [**UI**](#via-ui) | Fast setup without editing files | Go to `Collectors -> go.d -> SecretStores -> [[ entry.meta.kind ]]`, then add a secretstore. |
| [**File**](#via-file) | File-based configuration or automation | Edit `/etc/netdata/[[ entry.setup.configuration.file.name ]]` and add a `jobs` entry. |

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
{% details open=true summary="[[ entry.setup.configuration.options.folding.title or 'Config options' ]]" %}
[% endif %]

[% set has_groups = entry.setup.configuration.options.list | selectattr("group","defined") | list | length > 0 %]

[% if has_groups %]
| Group | Option | Description | Default | Required |
|:------|:-----|:------------|:--------|:---------:|
[% set ns = namespace(last_group=None) %]
[% for item in entry.setup.configuration.options.list %]
[% set anchor_source = (item.group ~ "-" ~ item.name) if (item.group is defined and item.group) else item.name %]
[% set item_anchor = "option-" ~ anchorfy(anchor_source) %]
| [[ ("**" ~ item.group ~ "**") if (item.group is defined and item.group != ns.last_group) else "" ]] | [[ ("[" ~ strfy(item.name) ~ "](#" ~ item_anchor ~ ")") if ('detailed_description' in item) else strfy(item.name) ]] | [[ strfy(item.description) ]] | [[ strfy(item.default_value) ]] | [[ strfy(item.required) ]] |
[% set ns.last_group = item.group if item.group is defined else ns.last_group %]
[% endfor %]
[% else %]
| Option | Description | Default | Required |
|:-----|:------------|:--------|:---------:|
[% for item in entry.setup.configuration.options.list %]
[% set anchor_source = (item.group ~ "-" ~ item.name) if (item.group is defined and item.group) else item.name %]
[% set item_anchor = "option-" ~ anchorfy(anchor_source) %]
| [[ ("[" ~ strfy(item.name) ~ "](#" ~ item_anchor ~ ")") if ('detailed_description' in item) else strfy(item.name) ]] | [[ strfy(item.description) ]] | [[ strfy(item.default_value) ]] | [[ strfy(item.required) ]] |
[% endfor %]
[% endif %]

[% for item in entry.setup.configuration.options.list %]
[% if 'detailed_description' in item %]
[% set anchor_source = (item.group ~ "-" ~ item.name) if (item.group is defined and item.group) else item.name %]
<a id="[[ "option-" ~ anchorfy(anchor_source) ]]"></a>
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
2. Go to `Collectors -> go.d -> SecretStores -> [[ entry.meta.kind ]]`.
3. Add a new secretstore and give it a store name.
4. Fill in the backend-specific settings.
5. Save the secretstore.

#### via File

Define the secretstore in `/etc/netdata/[[ entry.setup.configuration.file.name ]]`.

Each file contains a `jobs` array, and the secretstore kind is determined by the filename.

After editing the file, restart the Netdata Agent to load the updated secretstore definition.

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
