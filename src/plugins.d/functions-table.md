
> This document is a work in progress.

Plugin functions can support any kind of responses. However, the UI of Netdata has defined some structures as responses it can parse, understand and visualize.

One of these responses is the `table`. This is used in almost all functions implemented today.

# Functions Tables

Tables are defined when `"type": "table"` is set. The following is the standard header that should be available on all `table` responses:

```json
{
  "type": "table",
  "status": 200,
  "update_every": 1,
  "help": "help text",
  "hostname": "the hostname of the server sending this response, to appear at the title of the UI.",
  "expires": "UNIX epoch timestamp that the response expires",
  "has_history": "boolean: true when the datetime picker plays a role in the result set",
  // rest of the response
}
```

## Preflight `info` request

The UI, before making the first call to a function, it does a preflight request to understand what the function supports. The plugin receives this request as a FUNCTION call specifying the `info` parameter (possible among others).

The response from the plugin is expected to have the following:

```json
{
  // standard table header - as above
  "accepted_params": [ "a", "b", "c", ...],
  "required_params": [
    {
      "id": "the keyword to use when sending / receiving this parameter",
      "name": "the name to present to users for this parameter",
      "help": "a help string to help users understand this parameter",
      "type": "the type of the parameter, either: 'select' or 'multiselect'",
      "options": [
        {
          "id": "the keyword to use when sending / receiving this option",
          "name": "the name to present to users for this option",
          "pill": "a short text to show next to this option as a pill",
          "info": "a longer text to show on a tooltip when the user is hovering this option"
        },
        // more options for this required parameter
      ]
    },
    // more required parameters
  ]
}
```

If there are no required parameters, `required_params` can be omitted.
If there are no accepted parameters, `accepted_params` can be omitted. `accepted_param` can be sent during normal responses to update the UI with a new set of parameters available, between calls.

For `logs`, the UI requires this set of `accepted_params`.

Ref [Pagination](#pagination), [Deltas](#incremental-responses)
```json
[
  "info", // boolean: requests the preflight `info` request
  "after", // interval start timestamp
  "before", // interval end timestamp
  "direction", // sort direction [backward,forward]
  "last", // number of records to retrieve
  "anchor", // timestamp to divide records in pages
  "facets",
  "histogram", // selects facet to be used on the histogram
  "if_modified_since", // used in PLAY mode, to indicate that the UI wants data newer than the specified timestamp
  "data_only", // boolean: requests data (logs) only
  "delta", // boolean: requests incremental responses
  "tail",
  "sampling",
  "slice"
]
```

If there are `required_params`, the UI by default selects the first option. [](VERIFY_WITH_UI)

## Table data

To define table data, the UI expects this:

```json
{
  // header
  "columns": {
    "id": {
      "index": "number: the sort order for the columns, lower numbers are first",
      "name": "string: the name of the column as it should be presented to users",
      "unique_key": "boolean: true when the column uniquely identifies the row",
      "visible": "boolean: true when the column should be visible by default",
      "type": "enum: see column types",
      "units": "string: the units of the value, if any - this item can be omitted if the column does not have units [](VERIFY_WITH_UI)",
      "visualization": "enum: see visualization types",
      "value_options": {
        "units": "string: the units of the value  [](VERIFY_WITH_UI)",
        "transform": "enum: see transformation types",
        "decimal_points": "number: the number of fractional digits for the number",
        "default_value": "whatever the value is: when the value is null, show this instead"
      },
      "max": "number: when the column is numeric, this is the max value the data have - this is used when range filtering is set and value bars",
      "pointer_to": "id of another field: this is used when detail-string is set, to point to the column this column is detail of",
      "sort": "enum: sorting order",
      "sortable": "boolean: whether the column is sortable by users",
      "sticky": "boolean: whether the column should always be visible in the UI",
      "summary": "string: ???",
      "filter": "enum: the filtering type for this column",
      "full_width": "boolean: the value is expected to get most of the available column space. When multiple columns are full_width, the available space is given to all of them.",
      "wrap": "boolean: true when the entire value should be shown, even when it occupies a big space.",
      "default_expanded_filter": "boolean: true when the filter of this column should be expanded by default.",
      "dummy": "boolean: when set to true, the column is not to be presented to users."
    },
    // more IDs
  },
  "data": [ // array of rows
    [ // array of columns
      // values for each column linked to their "index" in the columns
    ],
    // next row
  ],
  "default_sort_column": "id: the id of the column that should be sorted by default"
}
```

**IMPORTANT**

On Data values, `timestamp` column value must be in unix micro.


### Sorting order

- `ascending`
- `descending`

### Transformation types

- `none`, just show the value, without any processing
- `number`, just show a number with its units, respecting `decimal_points`
- `duration`, makes the UI show a human readable duration, of the seconds given
- `datetime`, makes the UI show a human readable datetime of the timestamp in UNIX epoch
- `datetime_usec`, makes the UI show a human readable datetime of the timestamp in USEC UNIX epoch

### Visualization types

- `value`
- `bar`
- `pill`
- `richValue`, this is not used yet, it is supposed to be a structure that will provide a value and options for it
- `rowOptions`, defines options for the entire row - this column is hidden from the UI

### rowOptions

TBD

### Column types

- `none`
- `integer`
- `boolean`
- `string`
- `detail-string`
- `bar-with-integer`
- `duration`
- `timestamp`
- `array`

### Filter types

- `none`, this facet is not selectable by users
- `multiselect`, the user can select any number of the available options
- `facet`, similar to `multiselect`, but it also indicates that the column has been indexed and has values with counters. Columns set to `facet` must appear in the `facets` list.
- `range`, the user can select a range of values (numeric)

The plugin may send non visible columns with filter type `facet`. This means that the plugin can enable indexing on these columns, but it has not done it. Then the UI may send `facets:{ID1},{ID2},{ID3},...` to enable indexing of the columns specified.

What is the default?

#### Facets

Facets are a special case of `multiselect` fields. They are used to provide additional information about each possible value, including their relative sort order and the number of times each value appears in the result set. Facets are filters handled by the plugin. So, the plugin will receive user selected filter like: `{KEY}:{VALUE1},{VALUE2},...`, where `{KEY}` is the id of the column and `{VALUEX}` is the id the facet option the user selected.

```json
{
  // header,
  "columns": ...,
  "data": ...,
  "facets": [
    {
      "id": "string: the unique id of the facet",
      "name": "string: the human readable name of the facet",
      "order": "integer: the sorting order of this facet - lower numbers move items above others"
      "options": [
        {
          "id": "string: the unique id of the facet value",
          "name": "string: the human readable version of the facet value",
          "count": "integer: the number of times this value appears in the result set",
          "order": "integer: the sorting order of this facet value - lower numbers move items above others"
        },
        // next option
      ],
    },
    // next facet
  ]
}
```

## Charts

```json
{
  // header,
  "charts": {

  },
  "default_charts": [

  ]
}
```


## Histogram

```json
{
  "available_histograms": [
    {
      "id": "string: the unique id of the histogram",
      "name": "string: the human readable name of the histogram",
      "order": "integer: the sorting order of available histograms - lower numbers move items above others"
    }
  ],
  "histogram": {
    "id": "string: the unique id of the histogram",
    "name": "string: the human readable name of the histogram",
    "chart": {
      "summary": {
        "nodes": [
          {
            "mg": "string",
            "nm": "string: node name",
            "ni": "integer: node index"
          }
        ],
        "contexts": [
          {
            "id": "string: context id"
          }
        ],
        "instances": [
          {
            "id": "string: instance id",
            "ni": "integer: instance index"
          }
        ],
        "dimensions": [
          {
            "id": "string: dimension id",
            "pri": "integer",
            "sts": {
              "min": "float: dimension min value",
              "max": "float: dimension max value",
              "avg": "float: dimension avarage value",
              "arp": "float",
              "con": "float"
            }
          }
        ]
      },
      "result": {
        "labels": [
          // histogram labels
        ],
        "point": {
          "value": "integer",
          "arp": "integer",
          "pa": "integer"
        },
        "data": [
          [
            "timestamp" // unix milli
            // one array per label
            [
              // values
            ],
          ]
        ]
      },
      "view": {
        "title": "string: histogram tittle",
        "update_every": "integer",
        "after": "timestamp: histogram window start",
        "before": "timestamp: histogram window end",
        "units": "string: histogram units",
        "chart_type": "string: histogram chart type",
        "min": "integer: histogram min value",
        "max": "integer: histogram max value",
        "dimensions": {
          "grouped_by": [
            // "string: histogram grouped by",
          ],
          "ids": [
            // "string: histogram label id",
          ],
          "names": [
            // "string: histogram human readable label name",
          ],
          "colors": [],
          "units": [
            // "string: histogram label unit",
          ],
          "sts": {
            "min": [
              // "float: label min value",
            ],
            "max": [
              // "float: label max value",
            ],
            "avg": [
              // "float: label avarage value",
            ],
            "arp": [
              // "float",
            ],
            "con": [
              // "float",
            ]
          }
        }
      },
      "totals": {
        "nodes": {
          "sl": "integer",
          "qr": "integer"
        },
        "contexts":  {
          "sl": "integer",
          "qr": "integer"
        },
        "instances":  {
          "sl": "integer",
          "qr": "integer"
        },
        "dimensions":  {
          "sl": "integer",
          "qr": "integer"
        }
      },
      "db": {
        "update_every": "integer"
      }
    }
  }
}
```

**IMPORTANT**

On Result Data, `timestamps` must be in unix milli.

## Grouping

```json
{
  // header,
  "group_by": {

  }
}
```

## Datetime picker

When `has_history: true`, the plugin must accept `after:TIMESTAMP_IN_SECONDS` and `before:TIMESTAMP_IN_SECONDS` parameters.
The plugin can also turn pagination on, so that only a small set of the data are sent to the UI at a time.


## Pagination

The UI supports paginating results when `has_history: true`. So, when the result depends on the datetime picker and it is too big to be sent to the UI in one response, the plugin can enable datetime pagination like this:

```json
{
  // header,
  "columns": ...,
  "data": ...,
  "has_history": true,
  "pagination": {
    "enabled": "boolean: true to enable it",
    "column": "string: the column id that is used for pagination",
    "key": "string: the accepted_param that is used as the pagination anchor",
    "units": "enum: a transformation of the datetime picker to make it compatible with the anchor: timestamp, timestamp_usec"
  }
}
```

Once pagination is enabled, the plugin must support the following parameters:

- `{ANCHOR}:{VALUE}`, `{ANCHOR}` is the `pagination.key`, `{VALUE}` is the point the user wants to see entries at, formatted according to `pagination.units`.
- `direction:backward` or `direction:forward` to specify if the data to be returned if before are after the anchor.
- `last:NUMER`, the number of entries the plugin should return in the table data.
- `query:STRING`, the full text search string the user wants to search for.
- `if_modified_since:TIMESTAMP_USEC` and `tail:true`, used in PLAY mode, to indicate that the UI wants data newer than the specified timestamp. If there are no new data, the plugin must respond with 304 (Not Modified).

### Incremental Responses

- `delta:true` or `delta:false`, when the plugin supports incremental queries, it can accept the parameter `delta`. When set to true, the response of the plugin will be "added" to the previous response already available. This is used in combination with `if_modified_since` to optimize the amount of work the plugin has to do to respond.


### Other

- `slice:BOOLEAN` [](VERIFY_WITH_UI)
- `sampling:NUMBER`

