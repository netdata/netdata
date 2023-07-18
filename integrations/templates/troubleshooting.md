[% if entry.troubleshooting.list %]
## Troubleshooting

[% for item in entry.troubleshooting.list %]
### [[ item.name ]]

[[ description ]]

[% endfor %]
[% endif %]
