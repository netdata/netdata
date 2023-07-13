This directory contains templates used to generate the `integrations.js` file.

Templating is done using Jinja2 as a templating engine. Full documentation
can be found at https://jinja.palletsprojects.com/en/ (the ‘Template
Designer Documentation’ is the relevant part for people looking to
edit the templates, it’s not linked directly here to avoid embedding
version numbers in the links).

The particular instance of Jinja2 used has the following configuration
differences from the defaults:

- Any instances of curly braces in are replaced with square brackets
  (so instead of `{{ variable }}`, the syntax used here is `[[ variable
  ]]`. This is done so that templating commands for the frontend can be
  included without having to do any special escaping for them.
- `trim_blocks` and `lstrip_blocks` are both enabled, meaning that
  the first newline after a block will be _removed_, as will any leading
  whitespace on the same line as a block.

Each markdown template corresponds to the key of the same name in the
integrations objects in that file. Those templates get passed the
integration data using the name `entry`, plus the composed related
resource data using the name `rel_res`.

The `integrations.js` template is used to compose the final file. It gets
passed the JSON-formatted category and integration data using the names
`categories` and `integrations` respectively.
