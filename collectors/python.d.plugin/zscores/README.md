<!--
---
title: "zscores"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/zscores/README.md
---
-->

# Z-Scores - basic anomaly detection for your key metrics and charts

Smoothed, rolling [Z-Scores](https://en.wikipedia.org/wiki/Standard_score) for selected metrics or charts. 

This collector uses the netdata rest api to get the `mean` and `sigma` for each specified chart over a specified time range (defined by `train_secs` and `offset_secs`). For each dimension it will calculate a Z-Score as `z = (x - mean) / sigma` (clipped at `z_clip`). Scores are then smoothed over time (`z_smooth_n`) and, if `mode: 'per_chart'`, aggregated across dimensions (based on `per_chart_agg` function) to a smoothed, rolling chart level Z-Score at each time step.

## Charts

Below is an example of the charts produced by this collector and a typical example of how they would look when things are 'normal' on the system. The zscores tend to bounce randomly around a range typically between -3 to +3, one or two might stay steady at a more constant value depending on your configuration and the typical workload on your system. 

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-normal.jpg)

If we then go onto the system and run a command like `stress-ng --matrix 2 -t 2m` to create some stress, we see some charts begin to have zscores that jump outside the typical range between -3 to +3. When the absolute zscore for a chart is greater than 3 you will see a corresponding line appear on the `zscores.3sigma` chart to make it a bit clearer what charts might be worth looking at first.

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-abnormal.jpg)

Then as the issue passes the zscores should settle back down into their normal range again as they are calulcated in a rolling and smoothed way (as defined in your `zscores.conf` file). 

![alt text](https://github.com/andrewm4894/random/blob/master/images/netdata/netdata-zscores-collector-normal-again.jpg)

## Configuration

Enable the collector and restart Netdata.

```bash
cd /etc/netdata/
sudo ./edit-config python.d.conf
# Set `zscores: no` to `zscores: yes`
sudo service netdata restart
```

The configuration for the zscores collector defines how it will behave on your system and might take some experimentation with over time to set it optimally for your system. Below are some sensible defaults to get you started. 

Edit the `python.d/zscores.conf` configuration file using `edit-config` from the your agent's [config
directory](https://learn.netdata.cloud/guides/step-by-step/step-04#find-your-netdataconf-file), which is usually at `/etc/netdata`.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config python.d/zscores.conf
```

The default configuration should look something like this. Here you can see each parameter (with sane defaults) and some information about each one and what it does.

```bash
# what host to pull data from
host: '127.0.0.1:19999'
# what charts to pull data for, if undefined will look for all system.* charts
charts_in_scope: 'system.cpu,system.load,system.io,system.pgpgio,system.ram,system.net,system.ip,system.ipv6,system.processes,system.ctxt,system.idlejitter,system.intr,system.softirqs,system.softnet_stat'
# length of time to base calulcations off for mean and sigma
train_secs: 3600 # use last 1 hour to work out the mean and sigma for the zscore
# offset preceeding latest data to ignore when calculating mean and sigma
offset_secs: 300 # ignore last 5 minutes of data when calculating the mean and sigma
# recalculate the mean and sigma every n steps of the collector
train_every_n: 300 # recalculate mean and sigma every 5 minutes
# smooth the z score by averaging it over last n values
z_smooth_n: 15 # take a rolling average of the last 15 zscore values to reduce sensitivity to temporary 'spikes'
# cap absolute value of zscore (before smoothing) as some value for better stability
z_clip: 10 # cap each zscore at 10 so as to avoid really large individual zscores swamping any rolling average
# burn in period in which to initially calculate mean and sigma on every step
burn_in: 20 # on startup of the collector continually update the mean and sigma incase any gaps or inital calculations fail to return
# mode can be to get a zscore 'per_dim' or 'per_chart'
mode: 'per_chart' # 'per_chart' means individual dimension level smoothed zscores will be averaged again to one zscore per chart per time step
```

## Requirements

- This collector will only work with python 3 and requires the below python packages be installed.

```bash
# become netdata user
sudo su -s /bin/bash netdata
# install required packages
pip install numpy pandas requests netdata-pandas
```

## Notes

- Python 3 is required as the [`netdata-pandas`](https://github.com/netdata/netdata-pandas) package uses python async libraries ([asks](https://pypi.org/project/asks/) and [trio](https://pypi.org/project/trio/)) to make asynchronous calls to the netdata rest api to get the required data for each chart when calculating the mean and sigma.
- It may take a few hours or so for the collector to 'settle' into it's typical behaviour in terms of the scores you will see in the normal running of your system.
- The zscores are clipped to a range of between -10 to +10 so you will never see any scores outside this range. 
- The zscore you see for each chart when using `mode: 'per_chart'` as actually an average zscore accross all the dimensions on the underlying chart.
- As this collector does some calculation itself in python you may want to try it out first on a test or development system to get a sense for its performance characteristics. Most of the work in calculating the mean and sigma will be pushed down to the underlying Netdata C libraries via the rest api. But some data wrangling and calculations are then done using [Pandas](https://pandas.pydata.org/) and [Numpy](https://numpy.org/) within the collector itself.      