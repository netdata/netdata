[% if entry.metrics.profile_coverage %]

## Metrics

The built-in Prometheus profiles on this page define [[ entry.metrics.profile_coverage.chart_count ]] curated charts across
the primary and applicable supporting profiles.
The catalogue is generated from the same profile design and runtime chart contracts used by the Agent.

Eligible metrics that are not covered by a curated chart, including future exporter metrics, can still be collected through
the generic Prometheus autogeneration behavior. This catalogue describes curated profile coverage; it is not an allowlist of
every metric that the collector can render.

[% if clean %]
<details open data-prometheus-profile-catalog>
<summary>Curated profile coverage ([[ entry.metrics.profile_coverage.chart_count ]] charts)</summary>
[% else %]
<!-- prometheus-profile-catalog -->
{% details open=true summary="Curated profile coverage ([[ entry.metrics.profile_coverage.chart_count ]] charts)" %}
[% endif %]

[% for profile in entry.metrics.profile_coverage.profiles %]
[% set profile_chart_label = profile.chart_count ~ ' chart' ~ ('s' if profile.chart_count != 1 else '') %]
[% if clean %]
<details[% if profile.open %] open[% endif %] data-prometheus-profile>
<summary>[[ profile.title|e ]] — [[ profile_chart_label ]]</summary>
[% else %]
<!-- prometheus-profile-profile -->
{% details[% if profile.open %] open=true[% endif %] summary="[[ profile.title|e ]] — [[ profile_chart_label ]]" %}
[% endif %]

[[ profile.summary ]]

[% if profile.role == 'supporting' %]
**Supporting profile for [[ profile.supported_by|e ]].** [[ profile.activation ]]
[% endif %]

[% for family in profile.families recursive %]
[% set family_loop = loop %]
[% set family_chart_label = family.chart_count ~ ' chart' ~ ('s' if family.chart_count != 1 else '') %]
[% if clean %]
<details data-prometheus-profile-family>
<summary>[[ family.name|e ]] ([[ family_chart_label ]])</summary>
[% else %]
<!-- prometheus-profile-family -->
{% details summary="[[ family.name|e ]] ([[ family_chart_label ]])" %}
[% endif %]

[% for chart in family.charts %]
[% if clean %]
<details data-prometheus-profile-chart>
<summary>[[ chart.title|e ]]</summary>
[% else %]
<!-- prometheus-profile-chart -->
{% details summary="[[ chart.title|e ]]" %}
[% endif %]

- **Entity scope:** [[ chart.entity_scope ]]
- **Units:** `[[ chart.units ]]`
- **Dimensions:** [% for dimension in chart.dimensions|unique(case_sensitive=true, attribute='name') %]`[[ dimension.name ]]`[% if not loop.last %], [% endif %][% endfor %]

[% if clean %]
<details>
<summary>Source metric selectors ([[ chart.selectors|length ]])</summary>
[% else %]
{% details summary="Source metric selectors ([[ chart.selectors|length ]])" %}
[% endif %]

[% for selector in chart.selectors %]
- `[[ selector ]]`
[% endfor %]

[% if clean %]
</details>
[% else %]
{% /details %}
[% endif %]

[% if clean %]
</details>
[% else %]
{% /details %}
[% endif %]
[% endfor %]

[% if family.children %]
[[ family_loop(family.children) ]]
[% endif %]

[% if clean %]
</details>
[% else %]
{% /details %}
[% endif %]
[% endfor %]

[% if clean %]
</details>
[% else %]
{% /details %}
[% endif %]
[% endfor %]

[% if clean %]
</details>
[% else %]
{% /details %}
[% endif %]
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
