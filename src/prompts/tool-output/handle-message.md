{% comment %}
Variables:
- handle: tool output handle string
- bytes: number of bytes in stored output
- lines: number of lines in stored output
- tokens: token estimate for stored output
{% endcomment %}
Tool output is too large ({{ bytes }} bytes, {{ lines }} lines, {{ tokens }} tokens).
Call tool_output(handle = "{{ handle }}", extract = "what to extract").
The handle is a relative path under the tool_output root.
Provide precise and detailed instructions in `extract` about what you are looking for.
