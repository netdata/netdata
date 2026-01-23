[% if entry.functions and entry.functions.list %]
## Functions

[[ entry.functions.description ]]

[% for func in entry.functions.list %]
### [[ func.name ]]

[[ func.description ]]

| Aspect | Description |
|:-------|:------------|
| Name | `[[ strfy(entry.meta.module_name)|capitalize ]]:[[ strfy(func.id) ]]` |
| Performance | [[ strfy(func.performance) ]] |
| Security | [[ strfy(func.security) ]] |
| Availability | [[ strfy(func.availability) ]] |

#### Prerequisites

[% if func.prerequisites.list %]
[% for req in func.prerequisites.list %]
##### [[ req.title ]]

[[ req.description ]]

[% endfor %]
[% else %]
No additional configuration is required.
[% endif %]

#### Parameters

[% if func.parameters %]
| Parameter | Type | Description | Required | Default | Options |
|:---------|:-----|:------------|:--------:|:--------|:--------|
[% for param in func.parameters %]
| [[ strfy(param.name) ]] | [[ strfy(param.type) ]] | [[ strfy(param.description) ]] | [[ strfy(param.required) ]] | [[ strfy(param.default) ]] | [% if param.options %][% for opt in param.options %][[ strfy(opt.name) ]][% if opt.default %] (default)[% endif %][% if not loop.last %], [% endif %][% endfor %][% endif %] |
[% endfor %]
[% else %]
This function has no parameters.
[% endif %]

#### Returns

[[ func.returns.description ]]

| Column | Type | Unit | Visibility | Description |
|:-------|:-----|:-----|:-----------|:------------|
[% for col in func.returns.columns %]
| [[ strfy(col.name) ]] | [[ strfy(col.type) ]] | [[ strfy(col.unit) ]] | [[ strfy(col.visibility) ]] | [[ strfy(col.description) ]] |
[% endfor %]

[% endfor %]
[% endif %]
