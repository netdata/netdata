{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce
- final_report_locked: boolean (ignored here; always false)
- response_mode: 'agentic' | 'chat'
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% capture meta_reminder_short %}{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}{% endcapture %}
{% assign meta_reminder_short = meta_reminder_short | strip %}
{% capture final_report_how_to_provide %}{% if response_mode == 'agentic' %}using the XML wrapper tags{% else %}in your output (no XML wrapper){% endif %}{% endcapture %}
{% assign final_report_how_to_provide = final_report_how_to_provide | strip %}

### RESPONSE FORMAT RULES
{% if response_mode == 'agentic' %}- You operate in {{ response_mode }} mode interfacing with software with strict formatting requirements{% else %}- You operate in {{ response_mode }} mode, chatting directly to end users{% endif %}
- Your final report/answer MUST be provided {{ final_report_how_to_provide }}{% if meta_required %} ({{ meta_reminder_short }}){% endif %}
- If tools are available and you need to call them, do so

