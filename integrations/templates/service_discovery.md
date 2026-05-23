[# Jinja template: integrations/templates/service_discovery.md
   Renders the umbrella Service Discovery hub page (analog of templates/secrets.md).
   #]
[[ page.title ]]

[% for paragraph in page.intro %]
[[ paragraph ]]

[% endfor %]
### Jump To

[[ page.jump_to_line ]]


[[ page.how_it_works.heading ]]

[% for paragraph in page.how_it_works.intro %]
[[ paragraph ]]

[% endfor %]
[% for stage in page.how_it_works.stages %]
[[ loop.index ]]. **[[ stage.name ]]** — [[ stage.description ]]
[% endfor %]


[[ page.config_file.heading ]]

[% for paragraph in page.config_file.intro %]
[[ paragraph ]]

[% endfor %]
[[ page.config_file.skeleton ]]

[% for note in page.config_file.notes %]
- [[ note ]]
[% endfor %]


[[ page.rule_eval.heading ]]

[% for paragraph in page.rule_eval.intro %]
[[ paragraph ]]

[% endfor %]
[% for shape in page.rule_eval.shapes %]
- **[[ shape.name ]]** — [[ shape.description ]]
[% endfor %]

### Order matters

[% for line in page.rule_eval.ordering %]
[[ line ]]
[% endfor %]


[[ page.helpers.heading ]]

[% for paragraph in page.helpers.intro %]
[[ paragraph ]]

[% endfor %]

[[ page.helpers.go_builtins.heading ]]

[[ page.helpers.go_builtins.intro ]]

| Construct | Description |
|:----------|:------------|
[% for row in page.helpers.go_builtins.table %]
| [[ row.name ]] | [[ row.description ]] |
[% endfor %]


[[ page.helpers.sprig.heading ]]

[[ page.helpers.sprig.intro ]]

| Function | Description |
|:---------|:------------|
[% for row in page.helpers.sprig.table %]
| [[ row.name ]] | [[ row.description ]] |
[% endfor %]

[[ page.helpers.sprig.outro ]]


[[ page.helpers.netdata.heading ]]

[[ page.helpers.netdata.intro ]]

[% for row in page.helpers.netdata.table %]
- [[ row.name ]] — [[ row.description ]]
[% if row.sub is defined %]
[% for sub in row.sub %]
  - [[ sub ]]
[% endfor %]
[% endif %]
[% endfor %]

**Notes:**

[% for note in page.helpers.netdata.notes %]
- [[ note ]]
[% endfor %]


[[ page.config_template.heading ]]

[% for paragraph in page.config_template.intro %]
[[ paragraph ]]

[% endfor %]
[% for rule in page.config_template.rules %]
- **[[ rule.name ]]** — [[ rule.description ]]
[% endfor %]


[[ page.supported.heading ]]

[[ page.supported.intro ]]

| Discoverer | Kind | Stock conf | Discovers |
|:-----------|:-----|:-----------|:----------|
[% for d in discoverers %]
| [[ d.name_link ]] | `[[ d.kind ]]` | `[[ d.config_file ]]` | [[ d.tagline ]] |
[% endfor %]


[[ page.mixing.heading ]]

[% for paragraph in page.mixing.body %]
[[ paragraph ]]

[% endfor %]

[[ page.troubleshooting.heading ]]

[% for paragraph in page.troubleshooting.intro %]
[[ paragraph ]]

[% endfor %]
[% for item in page.troubleshooting.problems %]
### [[ item.name ]]

[[ item.description ]]

[% endfor %]
