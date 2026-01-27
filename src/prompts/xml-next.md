{% comment %}
Variables:
- nonce: XML nonce
- turn: current turn number
- max_turns: max turns (number or null)
- max_tools: max tools per turn (number or null)
- attempt: current attempt number
- max_retries: max retries
- context_percent_used: context percent used
- expected_final_format: output format id
- format_prompt_value: output format guidance string
- datetime: RFC3339 timestamp
- timestamp: unix timestamp seconds
- day: weekday name
- timezone: IANA timezone
- has_external_tools: boolean
- task_status_tool_enabled: boolean
- forced_final_turn_reason: string (context|max_turns|task_status_completed|task_status_only|retry_exhaustion|user_stop or empty)
- final_turn_tools: array of allowed tool names for final turn
- final_report_locked: boolean
- consecutive_progress_only_turns: number
- show_last_retry_reminder: boolean
- allow_router_handoff: boolean
- plugin_requirements: array of plugin requirement objects
- missing_meta_plugin_names: array of plugin names
{% endcomment %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% capture meta_reminder_short %}{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}{% endcapture %}
{% assign meta_reminder_short = meta_reminder_short | strip %}
{% capture meta_reminder_short_locked %}{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: true %}{% endcapture %}
{% assign meta_reminder_short_locked = meta_reminder_short_locked | strip %}
{% capture meta_xml_snippets %}{% render 'meta/xml-next-snippets.md', plugin_requirements: plugin_requirements %}{% endcapture %}
{% assign meta_xml_snippets = meta_xml_snippets | strip %}
{% assign final_slot_id = nonce | append: '-FINAL' %}
{% capture final_wrapper_example %}<ai-agent-{{ final_slot_id }} format="{{ expected_final_format }}">[final report/answer]</ai-agent-{{ final_slot_id }}>{% endcapture %}
{% assign final_wrapper_example = final_wrapper_example | strip %}
{% assign has_warnings = false %}
{% if final_report_locked %}
{% assign has_warnings = true %}
{% elsif forced_final_turn_reason != '' %}
{% assign has_warnings = true %}
{% elsif consecutive_progress_only_turns > 0 %}
{% assign has_warnings = true %}
{% endif %}

# System Notice

This is turn/step {{ turn }}{% if max_turns %} of {{ max_turns }}{% endif %}{% if attempt > 1 %}, attempt {{ attempt }} of {{ max_retries }}{% endif %}. Your context window is {{ context_percent_used }}% full.
Current date and time (RFC3339): {{ datetime }}
Unix epoch timestamp: {{ timestamp }}
Day: {{ day }}
Timezone: {{ timezone }}{% if has_external_tools and max_tools %}
You can execute up to {{ max_tools }} tools per turn.{% endif %}
{% if has_warnings %}

{% if final_report_locked %}
FINAL report already accepted. Do NOT resend the FINAL wrapper. Provide the required META wrappers now.
{% if missing_meta_plugin_names.size > 0 %}
Missing META plugins: {{ missing_meta_plugin_names | join: ', ' }}.
{% else %}
Missing META plugins were not identified; provide all required META blocks.
{% endif %}
{{ meta_reminder_short_locked }}
{% else %}
{% if forced_final_turn_reason == '' and consecutive_progress_only_turns > 0 %}
Turn wasted: you called task-status without any other tools and without providing a final report/answer, so a turn has been wasted without any progress on your task. CRITICAL: CALL task-status ONLY TOGETHER WITH OTHER TOOLS. Focus on making progress towards your final report/answer. Do not call task-status again.
{{ meta_reminder_short }}
(This is occurrence {{ consecutive_progress_only_turns }} of 5 before forced finalization)
{% endif %}
{% if forced_final_turn_reason != '' %}
{% capture final_turn_message %}
{% case forced_final_turn_reason %}
{% when 'context' %}You run out of context window. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.
{% when 'max_turns' %}You are not allowed to run more turns. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.
{% when 'task_status_completed' %}You marked the task-status as completed. You MUST now provide your final report/answer.
{% when 'task_status_only' %}You are repeatedly calling task-status without any other tools or a final report/answer and the system stopped you to prevent infinite loops. You MUST now provide your final report/answer, even if incomplete. If you have not completed the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.
{% when 'retry_exhaustion' %}All retry attempts have been exhausted. You MUST now provide your final report/answer, even if incomplete. If you have not finished the task, state this limitation in your final report/answer. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. Provide only what you have done so far and clearly state the fact that you have been stopped before completing the task, allowing the user to call you again to complete the task properly.
{% when 'user_stop' %}The user has requested to stop. You MUST now provide your final report/answer, summarizing your progress so far. If you have not finished the task, state clearly what was completed and what remains. **DO NOT FILL THE GAPS WITH ASSUMPTIONS OR GUESSES**. The user may call you again to complete the task.
{% else %}You MUST now provide your final report/answer.
{% endcase %}
{% endcapture %}
{% assign final_turn_message = final_turn_message | strip %}
{% if allow_router_handoff %}
{% assign final_turn_message = final_turn_message | append: " OR call `router__handoff-to` to hand off the user request to another agent." %}
{% endif %}
{{ final_turn_message }}
{{ meta_reminder_short }}
{% endif %}
{% endif %}
{% endif %}

{% if meta_required %}
{% assign relevant_names = plugin_requirements | map: 'name' %}
{% if final_report_locked and missing_meta_plugin_names.size > 0 %}
{% assign relevant_names = missing_meta_plugin_names %}
{% endif %}
{% if final_report_locked %}
## META Requirements — FINAL Already Accepted
The FINAL wrapper has already been accepted for this session. Do NOT resend it.
Missing META plugins: {{ relevant_names | join: ', ' }}.

Missing META wrappers (exact tags):
{% else %}
## META Requirements
META is mandatory with the FINAL wrapper in this session.

Required META wrappers (exact tags):
{% endif %}
{% for plugin in plugin_requirements %}
{% if relevant_names contains plugin.name %}
- <ai-agent-{{ nonce }}-META plugin="{{ plugin.name }}">{...}</ai-agent-{{ nonce }}-META>
{% endif %}
{% endfor %}

Plugin META instructions:
{{ meta_xml_snippets }}

{% endif %}
{% if forced_final_turn_reason != '' %}
{% if final_report_locked %}
FINAL already accepted. Do NOT resend the FINAL wrapper.
Provide the missing META wrappers now. Do NOT call tools.
{{ meta_reminder_short_locked }}
{% else %}
{% capture final_footer_message %}
You must now provide your final report/answer in the expected format ({{ expected_final_format }}) using the XML wrapper: {{ final_wrapper_example }}
{% endcapture %}
{% assign final_footer_message = final_footer_message | strip %}
{% if allow_router_handoff %}
{% assign final_footer_message = final_footer_message | append: " OR call `router__handoff-to` to hand off the user request to another agent." %}
{% endif %}
{{ final_footer_message }}
{% if meta_required %}
{{ meta_xml_snippets }}
{% endif %}
{{ meta_reminder_short }}
{% if final_turn_tools.size > 0 %}

Allowed tools for this final turn: {{ final_turn_tools | join: ', ' }}
{% endif %}
{% endif %}
{% else %}
{% if final_report_locked %}
FINAL already accepted. Do NOT resend the FINAL wrapper.
Provide the missing META wrappers now. Do NOT call tools.
{{ meta_reminder_short_locked }}
{% else %}
{% if has_external_tools %}
You now need to decide your next move:
EITHER
- Call tools to advance your task following the main prompt instructions (pay attention to tool formatting and schema requirements).
{% if task_status_tool_enabled %}
- Together with these tool calls, also call `agent__task_status` to inform your user about what you are doing and why you are calling these tools.
{% endif %}
OR
- Provide your final report/answer in the expected format ({{ expected_final_format }}) using the XML wrapper: {{ final_wrapper_example }}
{% if meta_required %}
{{ meta_xml_snippets }}
{% endif %}
{{ meta_reminder_short }}
{% else %}
You must now provide your final report/answer in the expected format ({{ expected_final_format }}) using the XML wrapper: {{ final_wrapper_example }}
{% if meta_required %}
{{ meta_xml_snippets }}
{% endif %}
{{ meta_reminder_short }}
{% endif %}
{% endif %}
{% endif %}

{% if show_last_retry_reminder %}
Reminder: do not end with plain text. Use an available tool to make progress. When ready to conclude, provide your final report/answer in the required XML wrapper.
{{ meta_reminder_short }}
{% endif %}
