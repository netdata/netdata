{% comment %}
Variables:
- entries: array of { slot_id, tool, status, duration_text, request, response }
{% endcomment %}
# System Notice

## Previous Turn Tool Responses

{% for entry in entries %}
<ai-agent-{{ entry.slot_id }} tool="{{ entry.tool }}" status="{{ entry.status }}"{% if entry.duration_text %}{{ entry.duration_text }}{% endif %}>
<request>
{{ entry.request }}
</request>
<response>
{{ entry.response }}
</response>
</ai-agent-{{ entry.slot_id }}>

{% endfor %}
