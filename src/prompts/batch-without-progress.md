{% comment %}
Variables:
- (none)
{% endcomment %}

#### agent__batch — How to Run Tools in Parallel

- Use this helper to execute multiple tools in one request.
- Each `calls[]` entry needs an `id`, the real tool name, and a `parameters` object that matches that tool's schema.
- Example:
  {
    "calls": [
      { "id": 1, "tool": "tool1", "parameters": { "param1": "value1", "param2": "value2" } },
      { "id": 2, "tool": "tool2", "parameters": { "param1": "value1" } }
      (Tool names must match exactly, and every required parameter must be present.)
    ]
  }

### MANDATORY RULE FOR PARALLEL TOOL CALLS
When gathering information from multiple independent sources, use the agent__batch tool to execute tools in parallel.
