<!--
title: "Lightweight Nvidia GPU monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/python.d.plugin/nvidia_smi_csv/README.md
sidebar_label: "Nvidia GPUs"
-->

# Lightweight Nvidia GPU monitoring with Netdata

Monitors limited metrics (product, temperature, power draw, gpu and memory utilization) using less resources using `nvidia-smi` cli tool.

> **Warning**: this collector does not work when the Netdata Agent is [running in a container](https://learn.netdata.cloud/docs/agent/packaging/docker).


## Requirements and Notes

-   You must have the `nvidia-smi` tool installed and your NVIDIA GPU(s) must support the tool. Mostly the newer high end models used for AI / ML and Crypto or Pro range, read more about [nvidia_smi](https://developer.nvidia.com/nvidia-system-management-interface).
-   You must enable this plugin, as its disabled by default due to minor performance issues:
    ```bash
    cd /etc/netdata   # Replace this path with your Netdata config directory, if different
    sudo ./edit-config python.d.conf
    ```
    Change the nvidia_smi_csv so it reads: `nvidia_smi_csv: yes`.
-   On some systems when the GPU is idle the `nvidia-smi` tool unloads and there is added latency again when it is next queried. If you are running GPUs under constant workload this isn't likely to be an issue. Alternatively, you may run the nvidia-persistenced service to prevent the GPU from entering into idle.
-   Currently the `nvidia-smi` tool is being queried via cli. Updating the plugin to use the nvidia c/c++ API directly should resolve this issue. See discussion here: <https://github.com/netdata/netdata/pull/4357>
-   Contributions are welcome.
-   Make sure `netdata` user can execute `/usr/bin/nvidia-smi` or wherever your binary is.

## Charts

It produces the following charts per GPU:

- GPU Utilization in `percentage`
- Memory Bandwidth Utilization in `percentage`
- Temperature in `celcius`
- Power Utilization in `Watts`

## Configuration

Configuration is not needed.
