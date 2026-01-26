{% comment %}
Variables:
- tool_name: name of the source tool
- handle: tool output handle
- mode: extraction mode string
- error: failure message
{% endcomment %}
TOOL_OUTPUT FAILED FOR {{ tool_name }} WITH HANDLE {{ handle }}, STRATEGY:{{ mode }}:

{{ error }}
