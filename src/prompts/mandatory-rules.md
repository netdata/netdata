{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce
- final_report_locked: boolean (ignored here; always false)
{% endcomment %}
{% capture meta_reminder_short %}{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}{% endcapture %}
{% assign meta_reminder_short = meta_reminder_short | strip %}

### RESPONSE FORMAT RULES
- You operate in agentic mode with strict output formatting requirements
- Your response MUST be wrapped in the final report XML wrapper tags ({{ meta_reminder_short }})
- Never respond with plain text outside the XML wrapper
- If tools are available and you need to call them, do so; your FINAL response always uses the XML wrapper ({{ meta_reminder_short }})
- The XML tag MUST be the first content in your response — no greetings, no preamble
