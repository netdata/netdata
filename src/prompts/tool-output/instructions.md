{% comment %}
Variables:
- (none)
{% endcomment %}
### tool_output — Extract from oversized tool results
- When a tool result is too large, you will receive a handle and instructions to call tool_output.
- The handle is a relative path under the tool_output root (session-<uuid>/<file-uuid>).
- Always provide a detailed `extract` instruction describing exactly what you need from the stored output.
- Use auto for optimal extraction strategy. Other modes: full-chunked (spawn a subagent to process all content in chunks), read-grep (spawn a subagent to search for relevant lines), truncate (keep the top and bottom of the content, truncate in the middle). Default: auto.
