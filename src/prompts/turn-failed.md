{% comment %}
Variables:
- events: array of { slug, reason }
- session_nonce: session XML nonce (string, may be empty)
- include_prefix: boolean (true to include "TURN-FAILED:" prefix)
- plugin_requirements: array of plugin requirement objects
- final_report_locked: boolean
{% endcomment %}
{% assign nonce = session_nonce %}
{% capture meta_reminder_short %}
{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: final_report_locked %}
{% endcapture %}
{% assign meta_reminder_short = meta_reminder_short | strip %}
{% capture reason_texts %}
{% for event in events %}
{% capture base %}
{% case event.slug %}
{% when 'final_report_format_mismatch' %}Final report format mismatch. Use the required format and resend your final report/answer.
{% when 'final_report_content_missing' %}Final report content is missing or empty. Provide non-empty content in the required format and resend your final report/answer.
{% when 'final_report_json_required' %}Final report must be a JSON object that matches the final-report/answer schema. Provide a JSON object (not a string) and retry.
{% when 'final_report_slack_messages_missing' %}Slack Block Kit final report is missing the messages array. Provide valid Block Kit messages and retry.
{% when 'final_report_schema_validation_failed' %}Final report failed schema validation. Fix the payload to match the final-report/answer schema and resend your final report/answer.
{% when 'final_meta_missing' %}Final report is incomplete: required META blocks are missing. FINAL already accepted; provide every required `<ai-agent-{{ nonce }}-META plugin="name">...</ai-agent-{{ nonce }}-META>` block with valid JSON. Do NOT resend the FINAL wrapper.
{% when 'final_meta_invalid' %}Final report META failed validation. FINAL already accepted; fix the META JSON for the specified plugin(s) and resend the required `<ai-agent-{{ nonce }}-META plugin="name">...</ai-agent-{{ nonce }}-META>` blocks only.
{% when 'tool_message_fallback_schema_failed' %}Fallback final report from tool output failed schema validation. Provide your valid final report/answer in the required format.
{% when 'content_without_tools_or_report' %}Text output detected without any tool calls or final report/answer. Call tool(s) or provide your final report/answer in the required wrapper exactly as instructed.
{% when 'empty_response' %}Empty response detected: no tool calls and no final report/answer were received. Call a tool or output your final report/answer in the required XML wrapper.
{% when 'reasoning_only' %}Reasoning-only output detected with no visible answer, tool calls, or final report. Call tools or provide your final report/answer in the required XML wrapper.
{% when 'reasoning_only_final' %}Reasoning-only output detected with no visible final report/answer. Relax. This is easy. You must now summarize and provide your final report/answer. Output now the XML wrapper exactly as instructed, and then summarize your work in the requested format.
{% when 'output_truncated' %}Output was truncated (stopReason=length). Retry with a shorter response that still includes the required tool calls or a complete final report.
{% when 'final_turn_no_report' %}Final turn ended without a valid final report. Provide the final report/answer now using the required XML wrapper and do not call any tools.
{% when 'xml_wrapper_as_tool' %}You called the XML wrapper tag as a tool, but it must be plain text in your response. Do NOT use tool-calling syntax; output the XML wrapper directly.
{% when 'xml_final_report_not_json' %}Final report payload is not valid JSON. Provide a JSON object that matches the final-report/answer schema and retry.
{% when 'xml_tool_payload_not_json' %}Tool payload is not valid JSON. Provide a JSON object for the tool parameters and retry.
{% when 'xml_slot_mismatch' %}XML wrapper ignored: the XML wrapper you used does not match the expected wrapper. Use the correct XML wrapper and retry.
{% when 'xml_missing_closing_tag' %}Malformed XML: missing closing tag. Close the tag exactly as shown and retry.
{% when 'xml_malformed_mismatch' %}Malformed XML: nonce/slot/tool mismatch or empty content. Use the exact nonce and slot, include non-empty content, and retry.
{% when 'xml_structured_output_truncated' %}Your response was truncated (stopReason=length) because it exceeded the output token limit. Retry with a shorter output that still delivers your complete final report/answer in the required XML wrapper.
{% else %}Turn failed due to an unknown error (slug={{ event.slug }}). Follow the latest instructions and retry.
{% endcase %}
{% endcapture %}
{% assign base = base | strip %}
{% if event.reason and event.reason != '' %}
{% assign base = base | append: ' Reason: ' | append: event.reason %}
{% endif %}
{% if forloop.first == false %} | {% endif %}{{ base }}
{% endfor %}
{% endcapture %}
{% if include_prefix %}
TURN-FAILED: {{ reason_texts | strip }}{% if meta_reminder_short and meta_reminder_short != '' %} {{ meta_reminder_short }}{% endif %}
{% else %}
{{ reason_texts | strip }}
{% endif %}
