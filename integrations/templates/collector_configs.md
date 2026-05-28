## Use in collector configs

[[ entry.collector_configs.description ]]

[% if entry.collector_configs.format.description %]
[[ entry.collector_configs.format.description ]]
[% endif %]

```text
[[ entry.collector_configs.format.syntax ]]
```

[% for part in entry.collector_configs.format.parts.list %]
- `[[ part.name ]]`: [[ part.description ]]
[% endfor %]

### Examples
[% for example in entry.collector_configs.examples.list %]
#### [[ example.name ]]

[[ example.description ]]

```[[ example.language or 'yaml' ]]
[[ example.content ]]
```
[% endfor %]
