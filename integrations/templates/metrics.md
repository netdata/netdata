[% if entry.metrics.profile_coverage %]

## Metrics

The built-in Prometheus profiles on this page map Prometheus metrics into
[[ entry.metrics.profile_coverage.chart_count ]] curated Netdata charts across the primary and applicable supporting profiles.
The tables are generated from the same profile design and runtime chart contracts used by the Agent.

Eligible metrics that are not covered by a curated chart, including future exporter metrics, can still be collected through
the generic Prometheus autogeneration behavior. This catalogue describes curated profile coverage; it is not an allowlist of
every metric that the collector can render.

[% for profile in entry.metrics.profile_coverage.profiles %]
### [[ profile.title|e ]]

[[ profile.summary ]]

[% if profile.role == 'supporting' %]
**Supporting profile for [[ profile.supported_by|e ]].** [[ profile.activation ]]
[% endif %]

[% for group in profile.metric_groups %]
#### [[ group.name|e ]]

| Prometheus metric | Netdata chart | Dimension | Unit | Scope |
|:------------------|:--------------|:----------|:-----|:------|
[% for row in group.rows %]
| <code>[[ row.prometheus_metric|markdown_table_cell ]]</code> | [[ row.netdata_chart|markdown_table_cell ]] | <code>[[ row.dimension|markdown_table_cell ]]</code> | <code>[[ row.unit|markdown_table_cell ]]</code> | [[ row.scope|markdown_table_cell ]] |
[% endfor %]

[% endfor %]
[% endfor %]
[% elif entry.metrics.scopes %]

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

[% set scope_has_description = scope.metrics|selectattr('description')|list|length > 0 %]
| Metric |[% if scope_has_description %] Description |[% endif %] Dimensions | Unit |[% for a in entry.metrics.availability %] [[ a ]] |[% endfor %]

|:------|[% if scope_has_description %]:------------|[% endif %]:----------|:----|[% for a in entry.metrics.availability %]:---:|[% endfor %]

[% for metric in scope.metrics %]
| [[ strfy(metric.name) ]] |[% if scope_has_description %] [[ strfy(metric.description)|e ]] |[% endif %] [% for d in metric.dimensions %][[ strfy(d.name) ]][% if not loop.last %], [% endif %][% endfor %] | [[ strfy(metric.unit) ]] |[% for a in entry.metrics.availability %] [% if not metric.availability|length or a in metric.availability %]•[% else %] [% endif %] |[% endfor %]

[% endfor %]

[% endfor %]
[% if entry.metrics.folding.enabled and not clean %]
{% /details %}
[% endif %]
[% else %]

## Metrics

[[ entry.metrics.description ]]
[% endif %]
