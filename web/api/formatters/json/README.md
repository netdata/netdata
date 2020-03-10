<!--
---
title: "JSON formatter"
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/api/formatters/json/README.md
---
-->

# JSON formatter

The CSV formatter presents [results of database queries](../../queries) in the following formats:

| format       | content type     | description|
|:----:|:----------:|:----------|
| `json`       | application/json | return the query result as a json object|
| `jsonp`      | application/json | return the query result as a JSONP javascript callback|
| `datatable`  | application/json | return the query result as a Google `datatable`|
| `datasource` | application/json | return the query result as a Google Visualization Provider `datasource` javascript callback|

The CSV formatter respects the following API `&options=`:

| option        | supported | description|
|:----:|:-------:|:----------|
| `google_json` | yes       | enable the Google flavor of JSON (using double quotes for strings and `Date()` function for dates|
| `objectrows`  | yes       | return each row as an object, instead of an array|
| `nonzero`     | yes       | to return only the dimensions that have at least a non-zero value|
| `flip`        | yes       | to return the rows older to newer (the default is newer to older)|
| `seconds`     | yes       | to return the date and time in unix timestamp|
| `ms`          | yes       | to return the date and time in unit timestamp as milliseconds|
| `percent`     | yes       | to replace all values with their percentage over the row total|
| `abs`         | yes       | to turn all values positive|
| `null2zero`   | yes       | to replace gaps with zeros (the default prints the string `null`|

## Examples

To show the differences between each format, in the following examples we query the same
chart (having just one dimension called `active`), changing only the query `format` and its `options`.

> Using `format=json` and `options=`

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&format=json&options='
{
 "labels": ["time", "active"],
    "data":
 [
      [ 1540644600, 224.2516667],
      [ 1540644000, 229.29],
      [ 1540643400, 222.41],
      [ 1540642800, 226.6816667],
      [ 1540642200, 246.4083333],
      [ 1540641600, 241.0966667]
  ]
}
```

> Using `format=json` and `options=objectrows`

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&format=json&options=objectrows'
{
 "labels": ["time", "active"],
    "data":
 [
      { "time": 1540644600, "active": 224.2516667},
      { "time": 1540644000, "active": 229.29},
      { "time": 1540643400, "active": 222.41},
      { "time": 1540642800, "active": 226.6816667},
      { "time": 1540642200, "active": 246.4083333},
      { "time": 1540641600, "active": 241.0966667}
  ]
}
```

> Using `format=json` and `options=objectrows,google_json`

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&formatjson&options=objectrows,google_json'
{
 "labels": ["time", "active"],
    "data":
 [
      { "time": new Date(2018,9,27,12,50,0), "active": 224.2516667},
      { "time": new Date(2018,9,27,12,40,0), "active": 229.29},
      { "time": new Date(2018,9,27,12,30,0), "active": 222.41},
      { "time": new Date(2018,9,27,12,20,0), "active": 226.6816667},
      { "time": new Date(2018,9,27,12,10,0), "active": 246.4083333},
      { "time": new Date(2018,9,27,12,0,0), "active": 241.0966667}
  ]
}
```

> Using `format=jsonp` and `options=`

```bash
curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&formjsonp&options='
callback({
 "labels": ["time", "active"],
    "data":
 [
      [ 1540645200, 235.885],
      [ 1540644600, 224.2516667],
      [ 1540644000, 229.29],
      [ 1540643400, 222.41],
      [ 1540642800, 226.6816667],
      [ 1540642200, 246.4083333]
  ]
});
```

> Using `format=datatable` and `options=`

```bash
curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&formdatatable&options='
{
 "cols":
 [
        {"id":"","label":"time","pattern":"","type":"datetime"},
        {"id":"","label":"","pattern":"","type":"string","p":{"role":"annotation"}},
        {"id":"","label":"","pattern":"","type":"string","p":{"role":"annotationText"}},
     {"id":"","label":"active","pattern":"","type":"number"}
  ],
    "rows":
 [
        {"c":[{"v":"Date(2018,9,27,13,0,0)"},{"v":null},{"v":null},{"v":235.885}]},
        {"c":[{"v":"Date(2018,9,27,12,50,0)"},{"v":null},{"v":null},{"v":224.2516667}]},
        {"c":[{"v":"Date(2018,9,27,12,40,0)"},{"v":null},{"v":null},{"v":229.29}]},
        {"c":[{"v":"Date(2018,9,27,12,30,0)"},{"v":null},{"v":null},{"v":222.41}]},
        {"c":[{"v":"Date(2018,9,27,12,20,0)"},{"v":null},{"v":null},{"v":226.6816667}]},
        {"c":[{"v":"Date(2018,9,27,12,10,0)"},{"v":null},{"v":null},{"v":246.4083333}]}
  ]
}
```

> Using `format=datasource` and `options=`

```bash
curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=nginx_local.connections&after=-3600&points=6&group=average&format=datasource&options='
google.visualization.Query.setResponse({version:'0.6',reqId:'0',status:'ok',sig:'1540645368',table:{
 "cols":
 [
        {"id":"","label":"time","pattern":"","type":"datetime"},
        {"id":"","label":"","pattern":"","type":"string","p":{"role":"annotation"}},
        {"id":"","label":"","pattern":"","type":"string","p":{"role":"annotationText"}},
     {"id":"","label":"active","pattern":"","type":"number"}
  ],
    "rows":
 [
        {"c":[{"v":"Date(2018,9,27,13,0,0)"},{"v":null},{"v":null},{"v":235.885}]},
        {"c":[{"v":"Date(2018,9,27,12,50,0)"},{"v":null},{"v":null},{"v":224.2516667}]},
        {"c":[{"v":"Date(2018,9,27,12,40,0)"},{"v":null},{"v":null},{"v":229.29}]},
        {"c":[{"v":"Date(2018,9,27,12,30,0)"},{"v":null},{"v":null},{"v":222.41}]},
        {"c":[{"v":"Date(2018,9,27,12,20,0)"},{"v":null},{"v":null},{"v":226.6816667}]},
        {"c":[{"v":"Date(2018,9,27,12,10,0)"},{"v":null},{"v":null},{"v":246.4083333}]}
  ]
}});
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fapi%2Fformatters%2Fjson%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
