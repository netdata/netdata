<!--
title: "Slurm queue monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/slurm/README.md
sidebar_label: "Slurm Queue"
-->

# Slurm queue monitoring with Netdata

Monitors slurm queue statistics using the squeue tool.  

Execute `squeue` to grab information from the slurm queue.

## Configuring slurm

Slurm monitoring is disabled by default. 

## Metrics and Alerts produced by this collector

| Chart      | Metrics     | Alert                    |
| ---------- | ----------- | ------------------------ |
| Slurm Queue | jobs | Triggers no alert |
