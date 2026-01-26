{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% if meta_required %}
{% for plugin in plugin_requirements %}
{{ plugin.finalReportExampleSnippet }}{% if forloop.last == false %}

{% endif %}
{% endfor %}
{% else %}
META examples: no META blocks are required for this session.
{% endif %}
