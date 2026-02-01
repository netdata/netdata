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
Your final report/answer is already received and accepted for this session. Provide required META wrappers only: {{ wrappers }}.
{% else %}
META REQUIRED AFTER YOUR FINAL REPORT/ANSWER. Exact META wrappers: {{ wrappers }}.
{% endif %}
{% else %}
META: none required for this session.
{% endif %}
