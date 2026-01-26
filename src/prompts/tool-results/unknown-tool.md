{% comment %}
Variables:
- tool_name: name of the unknown tool
{% endcomment %}
Unknown tool `{{ tool_name }}`: you called tool `{{ tool_name }}` but it does not match any of the tools in this session. Review carefully the tools available and copy the tool name verbatim. Tool names are made of a namespace (or tool provider) + double underscore + the tool name of this namespace/provider. When you call a tool, you must include both the namespace/provider and the tool name. You may now repeat the call to the tool, but this time you MUST supply the exact tool name as given in your list of tools.
