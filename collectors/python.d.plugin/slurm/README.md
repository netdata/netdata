<!--
title: "Slurm queue monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/slurm/README.md
sidebar_label: "Slurm Queue"
-->

# Slurm queue monitoring with Netdata

Monitors slurm queue statistics using the squeue tool.  

Execute `squeue -o %all` to grab squeue queue.

It produces only a single chart:

1.  **Slurm Queue**

    -   jobs

Configuration is not needed.

---


