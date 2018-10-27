# CSV formatter

The CSV formatter presents [results of database queries](../../queries) in the following formats:

format|content type|description
:---:|:---:|:-----
`csv`|text/plain|a text table, comma separated, with a header line (dimension names) and `\r\n` at the end of the lines
`csvjsonarray`|application/json|a JSON array, with each row as another array (the first row has the dimension names)
`tsv`|text/plain|like `csv` but TAB is used instead of comma to separate values (MS Excel flavor)
`html`|text/html|an html table
`markdown`|text/plain|markdown table

In all formats the date and time is the first column.

The CSV formatter respects the following API `&options=`:

option|supported|description
:---:|:---:|:---
`nonzero`|yes|to return only the dimensions that have at least a non-zero value
`flip`|yes|to return the rows older to newer (the default is newer to older)
`seconds`|yes|to return the date and time in unix timestamp
`ms`|yes|to return the date and time in unit timestamp as milliseconds
`percent`|yes|to replace all values with their percentage over the row total
`abs`|yes|to turn all values positive
`null2zero`|yes|to replace gaps with zeros (the default prints the string `null`


## Examples

Get the system total bandwidth for all physical network interfaces, over the last hour,
in 6 rows (one for every 10 minutes), in `csv` format:

Netdata always returns bandwidth in `kilobits`.

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=system.net&format=csv&after=-3600&group=sum&points=6&options=abs'
time,received,sent
2018-10-26 23:50:00,90214.67847,215137.79762
2018-10-26 23:40:00,90126.32286,238587.57522
2018-10-26 23:30:00,86061.22688,213389.23526
2018-10-26 23:20:00,85590.75164,206129.01608
2018-10-26 23:10:00,83163.30691,194311.77384
2018-10-26 23:00:00,85167.29657,197538.07773
```

---

Get the max RAM used by the SQL server and any cron jobs, over the last hour, in 2 rows (one for every 30
minutes), in `tsv` format, and format the date and time as unix timestamp:

Netdata always returns memory in `MB`.

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=apps.mem&format=tsv&after=-3600&group=max&points=2&options=nonzero,seconds&dimensions=sql,cron'
time	sql	cron
1540598400	61.95703	0.25
1540596600	61.95703	0.25
```

---

Get an HTML table of the last 4 values (4 seconds) of system CPU utilization:

Netdata always returns CPU utilization as `%`.

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=system.cpu&format=html&after=-4&options=nonzero'
<html>
<center>
<table border="0" cellpadding="5" cellspacing="5">
<tr><td>time</td><td>softirq</td><td>user</td><td>system</td></tr>
<tr><td>2018-10-27 00:16:07</td><td>0.25</td><td>1</td><td>0.75</td></tr>
<tr><td>2018-10-27 00:16:06</td><td>0</td><td>1.0025063</td><td>0.5012531</td></tr>
<tr><td>2018-10-27 00:16:05</td><td>0</td><td>1</td><td>0.75</td></tr>
<tr><td>2018-10-27 00:16:04</td><td>0</td><td>1.0025063</td><td>0.7518797</td></tr>
</table>
</center>
</html>
```

This is how it looks when rendered by a web browser:

![image](https://user-images.githubusercontent.com/2662304/47597887-bafbf480-d99c-11e8-864a-d880bb8d2e5b.png)


---

Get a JSON array with the average bandwidth rate of the mysql server, over the last hour, in 6 values
(one every 10 minutes), and return the date and time in milliseconds:

Netdata always returns bandwidth rates in `kilobits/s`.

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=mysql_local.net&format=csvjsonarray&after=-3600&points=6&group=average&options=abs,ms'
[
["time","in","out"],
[1540599600000,0.7499986,120.2810185],
[1540599000000,0.7500019,120.2815509],
[1540598400000,0.7499999,120.2812319],
[1540597800000,0.7500044,120.2819634],
[1540597200000,0.7499968,120.2807337],
[1540596600000,0.7499988,120.2810527]
]
``` 

---

Get the number of processes started per minute, for the last 10 minutes, in `markdown` format:

```bash
# curl -Ss 'https://registry.my-netdata.io/api/v1/data?chart=system.forks&format=markdown&after=-600&points=10&group=sum'
time|started
:---:|:---:
2018-10-27 03:52:00|245.1706149
2018-10-27 03:51:00|152.6654636
2018-10-27 03:50:00|163.1755789
2018-10-27 03:49:00|176.1574766
2018-10-27 03:48:00|178.0137076
2018-10-27 03:47:00|183.8306543
2018-10-27 03:46:00|264.1635621
2018-10-27 03:45:00|205.001551
2018-10-27 03:44:00|7026.9852167
2018-10-27 03:43:00|205.9904794
```

And this is how it looks when formatted:

time|started
:---:|:---:
2018-10-27 03:52:00|245.1706149
2018-10-27 03:51:00|152.6654636
2018-10-27 03:50:00|163.1755789
2018-10-27 03:49:00|176.1574766
2018-10-27 03:48:00|178.0137076
2018-10-27 03:47:00|183.8306543
2018-10-27 03:46:00|264.1635621
2018-10-27 03:45:00|205.001551
2018-10-27 03:44:00|7026.9852167
2018-10-27 03:43:00|205.9904794
