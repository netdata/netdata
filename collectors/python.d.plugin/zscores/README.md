<!--
---
title: "zscores"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/zscores/README.md
---
-->

# Z-Scores - basic anomaly detection for your key metrics and charts

Generate smoothed, rolling [Z-Scores](https://en.wikipedia.org/wiki/Standard_score) for selected metrics or charts. 

## Charts

Below is an example of the charts produced by this collector and a typical example of how they would look when things are 'normal' on the system. The zscores tend to bounce randomly around a range typically between -3 to +3, one or two might stay steady at a more constant value depending on your configuration and the typical workload on your system. 

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-normal.jpg)

If we then go onto the system and run a command like `stress-ng --matrix 2 -t 1m` to create some stress, we see some charts begin to have zscores that jump outside the typical range between -3 to +3. When the absolute zscore for a chart is greater than 3 you will see a corresponding line appear on the `zscores.3sigma` chart to make it a bit clearer what charts might be worth looking at first.

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-abnormal.jpg)

Then as the issue passes the zscores should settle back down into their normal range again as they are calulcated in a rolling and smoothed way (as defined in your `zscores.conf` file). 

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-normal-again.jpg)