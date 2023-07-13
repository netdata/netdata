[% if entry.troubleshooting.list|length > 0 %]
## Troubleshooting

[% for item in entry.troubleshooting.list %]
### [[ item.name ]]

[[ description ]]

[% endfor %]
[% endif %]
