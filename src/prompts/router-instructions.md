{% comment %}
Variables:
- destinations: array of agent ids
{% endcomment %}
Use router__handoff-to to route to one of: {{ destinations | join: ', ' }}
