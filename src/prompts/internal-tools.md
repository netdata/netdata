{% comment %}
Variables:
- format_id: output format id
- format_description: output format description
- expected_json_schema: JSON schema object (optional)
- slack_schema: Slack Block Kit schema object (optional)
- slack_mrkdwn_rules: Slack mrkdwn guidance (string)
- plugin_requirements: array of plugin requirement objects
- nonce: XML nonce
- response_mode: 'agentic' | 'chat'
- max_tool_calls_per_turn: max tools per turn
- batch_enabled: boolean
- progress_tool_enabled: boolean
- has_external_tools: boolean
- has_router_handoff: boolean
{% endcomment %}
{% capture final_report_block %}
{% render 'final-report.md',
  plugin_requirements: plugin_requirements,
  nonce: nonce,
  response_mode: response_mode,
  format_id: format_id,
  format_description: format_description,
  expected_json_schema: expected_json_schema,
  slack_schema: slack_schema,
  slack_mrkdwn_rules: slack_mrkdwn_rules
%}
{% endcapture %}
{{ final_report_block | strip }}

{% capture mandatory_rules_block %}
{% render 'mandatory-rules.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false, response_mode: response_mode %}
{% endcapture %}
{{ mandatory_rules_block | strip }}

{% assign has_any_tools = false %}
{% if progress_tool_enabled or batch_enabled or has_external_tools or has_router_handoff %}
{% assign has_any_tools = true %}
{% endif %}

{% if has_any_tools %}

### Tool Limits
{% if batch_enabled %}
- You can invoke at most {{ max_tool_calls_per_turn }} tools per turn/step (including those inside a batch request). Plan your tools accordingly.
{% else %}
- You can invoke at most {{ max_tool_calls_per_turn }} tools per turn/step.
{% endif %}
{% endif %}

{% if progress_tool_enabled or batch_enabled or has_router_handoff %}

### Internal Tools

The following internal tools are available. They expect valid JSON input according to their schemas.
{% if progress_tool_enabled %}

{% render 'task-status.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}
{% endif %}
{% if batch_enabled %}

{% if progress_tool_enabled %}
{% render 'batch-with-progress.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}
{% else %}
{% render 'batch-without-progress.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}
{% endif %}
{% endif %}
{% if has_router_handoff %}

#### router__handoff-to — Router Handoff
- This tool delegates the ORIGINAL user request to another agent.
- The destination agent will answer the user directly.
- You can include an optional message for the destination agent.
{% endif %}
{% endif %}
