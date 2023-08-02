[% if related %]
You can further monitor this integration by using:

[% for item in related %]
- {% relatedResource id="[[ item.id ]]" %}[[ item.name ]]{% /relatedResource %}: [[ item.info.description ]]
[% endfor %]
[% endif %]
