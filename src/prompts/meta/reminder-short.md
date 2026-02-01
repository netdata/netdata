{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce
- final_report_locked: boolean

Outputs nothing when no plugins are configured.
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% if meta_required %}
{% capture plugin_names %}{% for plugin in plugin_requirements %}{% if forloop.first == false %}, {% endif %}{{ plugin.name }}{% endfor %}{% endcapture %}
{% if final_report_locked %}
Your final report/answer is already received and accepted for this session. Provide required META wrappers only. Plugins: {{ plugin_names }}. Use <ai-agent-{{ nonce }}-META plugin="name">{...}</ai-agent-{{ nonce }}-META>.
{% else %}
META REQUIRED AFTER YOUR FINAL REPORT/ANSWER. Plugins: {{ plugin_names }}. Use <ai-agent-{{ nonce }}-META plugin="name">{...}</ai-agent-{{ nonce }}-META>.
{% endif %}
{% endif %}
