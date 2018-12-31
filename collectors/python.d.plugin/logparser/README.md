# logparser

This module is able to monitor an application specific log file and then create a chart based on occurrences of a log line (like amount of occurrences in a day, or a line graph of when the occurrences happen). It can read multiple files and produce multiple charts with multiple dimensions on each chart.

# Configuration


```yaml
chart_name:
    log_path: path/log/file
    dimensions:
      dimension: search_type=pattern
```

By default, the plugin does not read any files or produce any charts. To configure it, edit python.d/logparser.conf.

We have two different search types 
 - string : fetching metrics by finding string in each line of log file
 - regexp : fetching metrics by regexp pattern 

The sample config below shows the general definition of a chart named chart_name with a single dimension with name dimension_name. The metric for that dimension is a counter of the occurrences of lines matching the python regular expression regex_pattern in file path/logfile:

Above config show how to define your charts and how to fetching metrics from custom log files.
For each dimension in each chart must one regex be written in order to fetch those matches from that log file.It allows us to define whatever charts we want to show in dashboard.

A final config for more than one chart and more than one dimension could be something like this


```yaml

chart1_name:
    log_path: /path/logs/log.file
    dimensions:
      dimension_name1: 'string=GET'
      dimension_name2: 'string=POST'
      dimension_name3: 'string=PATCH'
chart2_name:
    log_path: /path/logs/log2.file
    dimensions:
      dimension_name1: 'regexp=[A-Z]+'
      dimension_name2: 'regexp=[A-Z]+'
      dimension_name3: 'regexp=[A-Z]+'

```


---
