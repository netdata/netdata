{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce
- final_report_locked: boolean
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% if meta_required %}
{% capture wrappers %}{% for plugin in plugin_requirements %}{% if forloop.first == false %} | {% endif %}<ai-agent-{{ nonce }}-META plugin="{{ plugin.name }}">{...}</ai-agent-{{ nonce }}-META>{% endfor %}{% endcapture %}
{% if final_report_locked %}
FINAL already accepted. Provide required META wrappers only: {{ wrappers }}.
{% else %}
META REQUIRED WITH FINAL. Exact META wrappers: {{ wrappers }}.
{% endif %}
{% else %}
META: none required for this session.
{% endif %}
