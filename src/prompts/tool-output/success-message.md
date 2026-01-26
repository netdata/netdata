{% comment %}
Variables:
- tool_name: name of the source tool
- handle: tool output handle
- mode: extraction mode string
- body: extracted content
{% endcomment %}
ABSTRACT FROM TOOL OUTPUT {{ tool_name }} WITH HANDLE {{ handle }}, STRATEGY:{{ mode }}:

{{ body }}
