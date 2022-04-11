<!--
title: "Slurm queue monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/slurm/README.md
sidebar_label: "Slurm Queue"
-->

# Slurm queue monitoring with Netdata

Monitors slurm queue statistics using the [`squeue`](https://slurm.schedmd.com/squeue.html) tool. Currently, only the number of pending and running jobs are collected.

## Configuring slurm

Slurm monitoring is disabled by default. 

### Example

```yaml
# job_name:
#     name: myname            # the JOB's name as it will appear at the
#                             # dashboard (by default is the job_name)
#                             # JOBs sharing a name are mutually exclusive
#     update_every: 1         # the JOB's data collection frequency
#     priority: 60000         # the JOB's order on the dashboard
#     penalty: yes            # the JOB's penalty
#     autodetection_retry: 0  # the JOB's re-check interval in seconds
```

## Metrics and Alerts produced by this collector

| Chart      | Metrics     | Alert                    |
| ---------- | ----------- | ------------------------ |
| Slurm Queue | jobs | Triggers no alert |
