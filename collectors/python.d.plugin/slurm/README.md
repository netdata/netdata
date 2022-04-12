<!--
title: "Slurm queue monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/slurm/README.md
sidebar_label: "Slurm Queue"
-->

# Slurm queue monitoring with Netdata

Monitors slurm queue statistics using the [`squeue`](https://slurm.schedmd.com/squeue.html) tool. Currently, only the number of pending and running jobs are collected.


## Configuring slurm

#### Prerequiistes
The slurm collector requires `squeue` to be installed and configured properly.

Slurm monitoring is disabled by default. To enable the collector:  
1. Navigate to the [Netdata config directory](https://learn.netdata.cloud/docs/configure/nodes#the-netdata-config-directory).
   ```bash
   cd /etc/netdata
   ```
2. Use the [`edit-config`](https://learn.netdata.cloud/docs/configure/nodes#use-edit-config-to-edit-configuration-files) script to edit `python.d.conf`.
   ```bash
   sudo ./edit-config python.d.conf
   ```
3. Enable the slurm collector by setting `slurm` to `yes`. 

   ```yaml
   slurm: yes
   ```
   
 4. Save the changes and restart the Agent with `sudo systemctl restart netdata` or the [appropriate method](https://learn.netdata.cloud/docs/configure/start-stop-restart) for your system.

### Example

As of now, the slurm plugin only has the default options, cf. [../README.md](../README.md).
For instance, you can increase the collection frequency from 10 seconds (the default) to 1 second by

```yaml
squeue:
    name: "Slurm Queue"     # the JOB's name as it will appear on the dashboard
    update_every: 1         # the JOB's data collection frequency (in seconds)
```

## Known issues
Please consider that too frequent updates can lead to performance issues with `squeue` as described [in the slurm documentation](https://slurm.schedmd.com/squeue.html#SECTION_PERFORMANCE).

## Metrics and Alerts produced by this collector

| Chart      | Metrics     | Alert                    |
| ---------- | ----------- | ------------------------ |
| Slurm Queue | jobs | Triggers no alert |
