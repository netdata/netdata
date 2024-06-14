[% if entry.metrics.scopes %]
## Metrics

[% if entry.metrics.folding.enabled and not clean %]
{% details open=true summary="[[ entry.metrics.folding.title ]]" %}
[% endif %]
Metrics grouped by *scope*.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

[[ entry.metrics.description ]]

[% for scope in entry.metrics.scopes %]
### Per [[ scope.name ]]

[[ scope.description ]]

[% if scope.labels %]
Labels:

| Label      | Description     |
|:-----------|:----------------|
[% for label in scope.labels %]
| [[ strfy(label.name) ]] | [[ strfy(label.description) ]] |
[% endfor %]
[% else %]
This scope has no labels.
[% endif %]

Metrics:

| Metric | Dimensions | Unit |[% for a in entry.metrics.availability %] [[ a ]] |[% endfor %]

|:------|:----------|:----|[% for a in entry.metrics.availability %]:---:|[% endfor %]

[% for metric in scope.metrics %]
| [[ strfy(metric.name) ]] | [% for d in metric.dimensions %][[ strfy(d.name) ]][% if not loop.last %], [% endif %][% endfor %] | [[ strfy(metric.unit) ]] |[% for a in entry.metrics.availability %] [% if not metric.availability|length or a in metric.availability %]•[% else %] [% endif %] |[% endfor %]

[% endfor %]

[% endfor %]
[% if entry.metrics.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% else %]
## Metrics

[[ entry.metrics.description ]]
[% endif %]
