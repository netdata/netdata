{% comment %}
Variables:
- plugin_requirements: array of plugin requirement objects
- nonce: static XML nonce
- final_report_locked: boolean (ignored here; always false)
{% endcomment %}
{% capture meta_reminder_short %}{% render 'meta/reminder-short.md', plugin_requirements: plugin_requirements, nonce: nonce, final_report_locked: false %}{% endcapture %}
{% assign meta_reminder_short = meta_reminder_short | strip %}

#### agent__batch — How to Run Tools in Parallel

- Use this helper to execute multiple tools in one request.
- Each `calls[]` entry needs an `id`, the real tool name, and a `parameters` object that matches that tool's schema.
- Example:
  {
    "calls": [
      { "id": 1, "tool": "agent__task_status", "parameters": { "status": "in-progress", "done": "Collected data about X", "pending": "researching Y and Z" } },
      { "id": 2, "tool": "tool1", "parameters": { "param1": "value1", "param2": "value2" } },
      { "id": 3, "tool": "tool2", "parameters": { "param1": "value1" } }
      (Tool names must match exactly, and every required parameter must be present.)
    ]
  }
- Do not combine `agent__task_status` with your final report; send the final report on its own. ({{ meta_reminder_short }})

### MANDATORY RULE FOR PARALLEL TOOL CALLS
When gathering information from multiple independent sources, use the agent__batch tool to execute tools in parallel.
- Include at most one task_status per batch
- task_status updates the user; to perform actions, use other tools in the same batch
- task_status can be called standalone to track task progress
