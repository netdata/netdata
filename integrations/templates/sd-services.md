[# Jinja template: integrations/templates/services.md
   Renders the SD-specific 'services:' rules section.
   - Heading is h2 'Service Rules' (sibling of Setup/Troubleshooting).
   - Shared template-helper reference lives on the SD hub page; this section
     only documents discoverer-specific variables.
   - Optional `services.evaluation` h3 surfaces rule-evaluation gotchas where
     they differ between discoverers (e.g. skip-rules in net_listeners/docker). #]
## Service Rules

[[ entry.services.description ]]

[% if entry.services.evaluation is defined and (entry.services.evaluation.description or entry.services.evaluation.list) %]
### How rules are evaluated
[% if entry.services.evaluation.description %]

[[ entry.services.evaluation.description ]]

[% endif %]
[% if entry.services.evaluation.list %]

[% for step in entry.services.evaluation.list %]
- **[[ step.name ]]** — [[ strfy(step.description) ]]
[% endfor %]

[% endif %]
[% endif %]
### Template Variables
[% if entry.services.template_variables.description %]

[[ entry.services.template_variables.description ]]

[% endif %]
[% set has_types = entry.services.template_variables.list | selectattr("type","defined") | list | length > 0 %]
[% if has_types %]

| Variable | Type | Description |
|:---------|:-----|:------------|
[% for v in entry.services.template_variables.list %]
| `[[ v.name ]]` | [[ v.type if v.type is defined else "string" ]] | [[ strfy(v.description) ]] |
[% endfor %]
[% else %]

| Variable | Description |
|:---------|:------------|
[% for v in entry.services.template_variables.list %]
| `[[ v.name ]]` | [[ strfy(v.description) ]] |
[% endfor %]
[% endif %]

### Examples
[% if entry.services.examples.description %]

[[ entry.services.examples.description ]]

[% endif %]
[% for example in entry.services.examples.list %]
#### [[ example.name ]]

[[ example.description ]]

```yaml
[[ example.config ]]
```

[% endfor %]
