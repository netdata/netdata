{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce

Outputs nothing when no plugins are configured.
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% if meta_required %}
### META Requirements (Mandatory With FINAL)
Your task is complete only when BOTH of the following are true:
- You output the FINAL wrapper with the final report/answer.
- You output ALL required META wrappers listed below.

Required META wrappers (exact tags):
{% for plugin in plugin_requirements %}
- <ai-agent-{{ nonce }}-META plugin="{{ plugin.name }}">{...}</ai-agent-{{ nonce }}-META>
{% endfor %}

{% for plugin in plugin_requirements %}
#### META Plugin: {{ plugin.name }}
Required wrapper: <ai-agent-{{ nonce }}-META plugin="{{ plugin.name }}">{...}</ai-agent-{{ nonce }}-META>

System instructions:
{{ plugin.systemPromptInstructions }}

Schema (must match exactly):
```json
{{ plugin.schema | json_pretty }}
```

{% endfor %}
{% endif %}
