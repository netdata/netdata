{% comment %}
Variables:
- tool_name: name of the source tool
- tool_args_json: JSON string of tool arguments
- handle: handle filename
- extract: extraction instruction text
{% endcomment %}
WHAT TO EXTRACT
{{ extract }}

FROM WHERE TO EXTRACT IT
The handle file is named `{{ handle }}`, and you have direct access to it via your tools `tool_output_fs__Read` and `tool_output_fs__Grep`. Both tools accept a filename. Pass this filename to them.

ADDITIONAL CONTEXT
Use the following information ONLY for context:
The handle file has been created from an oversized output of a tool called `{{ tool_name }}` of another agent.
That tool was run with parameters: {{ tool_args_json }}

WHAT IS EXPECTED FROM YOU
You are expected to use your tools (`tool_output_fs__Read` and `tool_output_fs__Grep`) to find relevant and potentially useful information and provide your findings with your final report/answer.

WHAT TO REPORT IF YOU FIND NOTHING RELEVANT
If you can't find anything relevant, your final report must start with: NO RELEVANT DATA FOUND
Then provide a short description of what kind of information is available in the handle file.

IMPORTANT LIMITATION
You operate in an isolated environment with access only to the handle file specified above. If the handle file contains references to other files or tools, you cannot access them. Focus exclusively on extracting information from the handle file content itself.
