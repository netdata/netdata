<!--
title: "Query formatting"
custom_edit_url: https://github.com/netdata/netdata/edit/master/web/api/formatters/README.md
-->

# Query formatting

API data queries need to be formatted before returned to the caller.
Using API parameters, the caller may define the format he/she wishes to get back.

The following formats are supported:

| format|module|content type|description|
|:----:|:----:|:----------:|:----------|
| `array`|[ssv](https://github.com/netdata/netdata/blob/master/web/api/formatters/ssv/README.md)|application/json|a JSON array|
| `csv`|[csv](https://github.com/netdata/netdata/blob/master/web/api/formatters/csv/README.md)|text/plain|a text table, comma separated, with a header line (dimension names) and `\r\n` at the end of the lines|
| `csvjsonarray`|[csv](https://github.com/netdata/netdata/blob/master/web/api/formatters/csv/README.md)|application/json|a JSON array, with each row as another array (the first row has the dimension names)|
| `datasource`|[json](https://github.com/netdata/netdata/blob/master/web/api/formatters/json/README.md)|application/json|a Google Visualization Provider `datasource` javascript callback|
| `datatable`|[json](https://github.com/netdata/netdata/blob/master/web/api/formatters/json/README.md)|application/json|a Google `datatable`|
| `html`|[csv](https://github.com/netdata/netdata/blob/master/web/api/formatters/csv/README.md)|text/html|an html table|
| `json`|[json](https://github.com/netdata/netdata/blob/master/web/api/formatters/json/README.md)|application/json|a JSON object|
| `jsonp`|[json](https://github.com/netdata/netdata/blob/master/web/api/formatters/json/README.md)|application/json|a JSONP javascript callback|
| `markdown`|[csv](https://github.com/netdata/netdata/blob/master/web/api/formatters/csv/README.md)|text/plain|a markdown table|
| `ssv`|[ssv](https://github.com/netdata/netdata/blob/master/web/api/formatters/ssv/README.md)|text/plain|a space separated list of values|
| `ssvcomma`|[ssv](https://github.com/netdata/netdata/blob/master/web/api/formatters/ssv/README.md)|text/plain|a comma separated list of values|
| `tsv`|[csv](https://github.com/netdata/netdata/blob/master/web/api/formatters/csv/README.md)|text/plain|a TAB delimited `csv` (MS Excel flavor)|

For examples of each format, check the relative module documentation.

## Metadata with the `jsonwrap` option

All data queries can be encapsulated to JSON object having metadata about the query and the results.

This is done by adding the `options=jsonwrap` to the API URL (if there are other `options` append
`,jsonwrap` to the existing ones).

This is such an object:

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=system.cpu&after=-3600&points=6&group=average&format=csv&options=nonzero,jsonwrap'
{
   "api": 1,
   "id": "system.cpu",
   "name": "system.cpu",
   "view_update_every": 600,
   "update_every": 1,
   "first_entry": 1540387074,
   "last_entry": 1540647070,
   "before": 1540647000,
   "after": 1540644000,
   "dimension_names": ["steal", "softirq", "user", "system", "iowait"],
   "dimension_ids": ["steal", "softirq", "user", "system", "iowait"],
   "latest_values": [0, 0.2493766, 1.745636, 0.4987531, 0],
   "view_latest_values": [0.0158314, 0.0516506, 0.866549, 0.7196127, 0.0050002],
   "dimensions": 5,
   "points": 6,
   "format": "csv",
   "result": "time,steal,softirq,user,system,iowait\n2018-10-27 13:30:00,0.0158314,0.0516506,0.866549,0.7196127,0.0050002\n2018-10-27 13:20:00,0.0149856,0.0529183,0.8673155,0.7121144,0.0049979\n2018-10-27 13:10:00,0.0137501,0.053315,0.8578097,0.7197613,0.0054209\n2018-10-27 13:00:00,0.0154252,0.0554688,0.899432,0.7200638,0.0067252\n2018-10-27 12:50:00,0.0145866,0.0495922,0.8404341,0.7011141,0.0041688\n2018-10-27 12:40:00,0.0162366,0.0595954,0.8827475,0.7020573,0.0041636\n",
 "min": 0,
 "max": 0
}
```

## Downloading data query result files

Following the [Google Visualization Provider guidelines](https://developers.google.com/chart/interactive/docs/dev/implementing_data_source),
Netdata supports parsing `tqx` options.

Using these options, any Netdata data query can instruct the web browser to download
the result and save it under a given filename.

For example, to download a CSV file with CPU utilization of the last hour,
[click here](https://registry.my-netdata.io/api/v1/data?chart=system.cpu&after=-3600&format=csv&options=nonzero&tqx=outFileName:system+cpu+utilization+of+the+last_hour.csv).

This is done by appending `&tqx=outFileName:FILENAME` to any data query.
The output will be in the format given with `&format=`.


