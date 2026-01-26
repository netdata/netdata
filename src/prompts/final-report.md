{% comment %}
Variables:
- nonce: static XML nonce
- format_id: output format id
- format_description: output format description
- expected_json_schema: JSON schema object (optional)
- slack_schema: Slack Block Kit schema object (optional)
- slack_mrkdwn_rules: Slack mrkdwn guidance (string)
- plugin_requirements: array of { name, schema, systemPromptInstructions, xmlNextSnippet, finalReportExampleSnippet }
{% endcomment %}
{% assign slot_id = nonce | append: '-FINAL' %}
{% assign meta_required = plugin_requirements.size > 0 %}
{% capture meta_detailed_block %}{% render 'meta/detailed-block.md', plugin_requirements: plugin_requirements, nonce: nonce %}{% endcapture %}
{% assign meta_detailed_block = meta_detailed_block | strip %}
{% capture meta_example_snippets %}{% render 'meta/example-snippets.md', plugin_requirements: plugin_requirements %}{% endcapture %}
{% assign meta_example_snippets = meta_example_snippets | strip %}
{% capture example_content %}
{% if format_id == 'slack-block-kit' %}
[ { "blocks": [ ... ] } ]
{% elsif format_id == 'json' %}
{ ... your JSON here ... }
{% else %}
[Your final report/answer here]
{% endif %}
{% endcapture %}
{% assign example_content = example_content | strip %}
{% capture schema_block %}
{% if format_id == 'json' and expected_json_schema %}
**Your response must be a JSON object matching this schema:**
```json
{{ expected_json_schema | json_pretty }}
```
{% elsif format_id == 'slack-block-kit' and slack_schema %}
**Your response must be a JSON array (the messages array directly, NOT wrapped in an object):**
```json
{{ slack_schema | json_pretty }}
```
{% endif %}
{% endcapture %}
{% assign schema_block = schema_block | strip %}
{% capture slack_mrkdwn_guidance %}
{% if format_id == 'slack-block-kit' %}
{{ slack_mrkdwn_rules }}
{% endif %}
{% endcapture %}
{% assign slack_mrkdwn_guidance = slack_mrkdwn_guidance | strip %}

## MANDATORY READ-FIRST: How to Provide Your Final Report/Answer

You must provide your final report/answer inside the XML wrapper shown below.
- All content must be between the opening and closing tags.
- If you could not complete the task, state the limitation clearly in the final report/answer.
{% if meta_required %}
- META is required for this session. Provide META wrappers immediately after the FINAL wrapper.
{% endif %}

**Required XML Wrapper:**
```
<ai-agent-{{ slot_id }} format="{{ format_id }}">
{{ example_content }}
</ai-agent-{{ slot_id }}>
```
{% if meta_required %}
{{ meta_example_snippets }}
{% endif %}

**Output Format: {{ format_id }}**
{{ format_description }}
{% if schema_block != '' %}
{{ schema_block }}
{% endif %}
{% if slack_mrkdwn_guidance != '' %}
{{ slack_mrkdwn_guidance }}
{% endif %}

{{ meta_detailed_block }}

### Examples

<example>
User: Hello
Assistant: <ai-agent-{{ slot_id }} format="{{ format_id }}">Hi! How can I help you today?</ai-agent-{{ slot_id }}>
{% if meta_required %}
{{ meta_example_snippets }}
{% endif %}
</example>

<example>
User: Research information about customer Acme Corp
Assistant: I'll help you find all information about Acme Corp.
Uses its tools to find information about Acme Corp.
When the research completes:
Assistant:
<ai-agent-{{ slot_id }} format="{{ format_id }}">
detailed information about Acme Corp in the right format
</ai-agent-{{ slot_id }}>
{% if meta_required %}
{{ meta_example_snippets }}
{% endif %}
</example>

<example>
User: Research the ARR change in the last 30 days
Assistant: I'll help you find how the ARR changed in the last 30 days.
Uses its tools to find how the ARR changed in the last 30 days.
When the search completes:
Assistant:
<ai-agent-{{ slot_id }} format="{{ format_id }}">
detailed information on how the ARR changed in the last 30 days, in the right format
</ai-agent-{{ slot_id }}>
{% if meta_required %}
{{ meta_example_snippets }}
{% endif %}
</example>

<example>
User: Find the meeting we had with John Smith last week and provide a summary of the discussion
Assistant: I'll help you get a summary of last week's discussion with John Smith.
Uses its tools to find the meeting with John Smith last week.
During the research the assistant runs out of turns and it is forced to stop and provide its final report/answer prematurely.
Assistant:
<ai-agent-{{ slot_id }} format="{{ format_id }}">
detailed information on what has been extracted from the meeting with John Smith last week, in the right format, with a prominent note that the research has been interrupted and may be incomplete.
</ai-agent-{{ slot_id }}>
{% if meta_required %}
{{ meta_example_snippets }}
{% endif %}
</example>
