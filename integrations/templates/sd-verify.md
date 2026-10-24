[# Jinja template: integrations/templates/verify.md
   Renders the optional '## Verify discovery worked' h2.
   Skip the section entirely if the metadata has no `verify:` block. #]
[% if entry.verify is defined and entry.verify.checks and entry.verify.checks.list %]
## Verify discovery worked
[% if entry.verify.description %]

[[ entry.verify.description ]]

[% endif %]
[% for check in entry.verify.checks.list %]
### [[ check.name ]]

[[ check.description ]]

[% endfor %]
[% endif %]
