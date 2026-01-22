[% if entry.functions and entry.functions.list %]
## Functions

[[ entry.functions.description ]]

[% for func in entry.functions.list %]
### [[ func.name ]]

| Aspect | Description |
|:-------|:------------|
| Name | [[ entry.meta.module_name|capitalize ]]:[[ func.id ]] |
| Summary | [[ strfy(func.summary) ]] |
| Behavior | [[ strfy(func.behavior) ]] |
| Performance | [[ strfy(func.performance) ]] |
| Security | [[ strfy(func.security) ]] |
| Requirements | [[ strfy(func.requirements) ]] |
| Availability | [[ strfy(func.availability) ]] |

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

| Column | Type | Description |
|:-------|:-----|:------------|
[% for col in func.returns.columns %]
| [[ strfy(col.name) ]] | [[ strfy(col.type) ]] | [[ strfy(col.description) ]] |
[% endfor %]

[% endfor %]
[% endif %]
