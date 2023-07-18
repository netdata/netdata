[% if related %]
You can further monitor this integration by using:

[% for item in related %]
- **[[ item.name ]]**: [[ item.info.description ]]
[% endfor %]
[% endif %]
