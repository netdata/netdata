<!--
title: "Slurm queue monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/slurm/README.md
sidebar_label: "Slurm Queue"
-->

# Slurm queue monitoring with Netdata

Monitors slurm queue statistics using the [`squeue`](https://slurm.schedmd.com/squeue.html) tool. Currently, only the number of pending and running jobs are collected.

Notice that this plugin requires `squeue` to be installed and configured properly.

## Configuring slurm

Slurm monitoring is disabled by default. It may be enabled by modifying [../python.d.conf](../python.d.conf) from

```yaml
slurm: no
```

to

```yaml
slurm: yes
```

### Example

As of now, the slurm plugin only has the default options, cf. [../README.md](../README.md).
For instance, you can increase the collection frequency from 10 seconds (the default) to 1 second by

```yaml
squeue:
    name: "Slurm Queue"     # the JOB's name as it will appear on the dashboard
    update_every: 1         # the JOB's data collection frequency (in seconds)
```

Please consider that too frequent updates can lead to performance issues with `squeue` as described [here](https://slurm.schedmd.com/squeue.html#SECTION_PERFORMANCE).

## Metrics and Alerts produced by this collector

| Chart      | Metrics     | Alert                    |
| ---------- | ----------- | ------------------------ |
| Slurm Queue | jobs | Triggers no alert |
